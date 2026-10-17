// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattermost/mattermost/server/public/model"
)

// The todos of a list: the open ones in their order, then the done ones, newest first. Someone
// assigned a todo of a channel's list finds it in their "Assigned to you" (chassigned_<user> lists
// where those todos are).

const (
	// AssignedListKey is the name of the refreshed list when someone's assignments change.
	AssignedListKey = "assigned"

	storeAssignedKey = "chassigned"

	maxOpenItems     = 500
	maxDoneItems     = 200
	maxMessageLength = 1000
	maxDescription   = 4000
)

var dueFormat = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Item is a todo of a list.
type Item struct {
	ID          string `json:"id"`
	ListID      string `json:"list_id"`
	Message     string `json:"message"`
	Description string `json:"description,omitempty"`
	CreatorID   string `json:"creator_id"`
	CreateAt    int64  `json:"create_at"`
	UpdateAt    int64  `json:"update_at"`
	// AssigneeID is a member of the list's channel the todo is assigned to.
	AssigneeID string `json:"assignee_id,omitempty"`
	// Due is the day it's due (YYYY-MM-DD), in the reader's time zone.
	Due           string `json:"due,omitempty"`
	DoneBy        string `json:"done_by,omitempty"`
	DoneAt        int64  `json:"done_at,omitempty"`
	PostID        string `json:"post_id,omitempty"`
	PostPermalink string `json:"post_permalink,omitempty"`
	// FromUserID and FromIssueID are, for a todo someone sent and the owner accepted, the sender
	// and their copy of it ("Sent by you"), which leaves their list once it's done.
	FromUserID  string `json:"from_user_id,omitempty"`
	FromIssueID string `json:"from_issue_id,omitempty"`
}

// Done tells whether the todo is checked off.
func (t *Item) Done() bool { return t.DoneAt != 0 }

// assignedRef points to a todo assigned to someone. Those of before lists have no list: theirs is
// their channel's default list (the one the migration made of the channel's todos).
type assignedRef struct {
	ChannelID string `json:"channel_id"`
	ListID    string `json:"list_id,omitempty"`
	TodoID    string `json:"todo_id"`
}

func (r assignedRef) listID() string {
	if r.ListID != "" {
		return r.ListID
	}
	return defaultChannelListID(r.ChannelID)
}

// AssignedTodo is a todo assigned to the person asking, with where it is.
type AssignedTodo struct {
	Item
	ChannelID      string `json:"channel_id"`
	ListName       string `json:"list_name"`
	ListIcon       string `json:"list_icon,omitempty"`
	ChannelName    string `json:"channel_name"`
	ChannelURLName string `json:"channel_url_name"`
	TeamName       string `json:"team_name,omitempty"`
}

type itemRequest struct {
	Message     *string `json:"message"`
	Description *string `json:"description"`
	AssigneeID  *string `json:"assignee_id"`
	Due         *string `json:"due"`
	Done        *bool   `json:"done"`
	// Position moves an open todo to that index among the open ones.
	Position *int   `json:"position"`
	PostID   string `json:"post_id"`
	// ChannelID is the channel the todo is added from, to offer its list first there next time.
	ChannelID string `json:"channel_id"`
}

func assignedKey(userID string) string { return storeAssignedKey + "_" + userID }

// sortItems keeps the open todos in their order, then the done ones, newest first, keeping at most
// maxDoneItems of them.
func sortItems(items []*Item) []*Item {
	open := make([]*Item, 0, len(items))
	var done []*Item
	for _, t := range items {
		if t.Done() {
			done = append(done, t)
		} else {
			open = append(open, t)
		}
	}
	slices.SortStableFunc(done, func(a, b *Item) int {
		switch {
		case a.DoneAt > b.DoneAt:
			return -1
		case a.DoneAt < b.DoneAt:
			return 1
		}
		return 0
	})
	if len(done) > maxDoneItems {
		done = done[:maxDoneItems]
	}
	return append(open, done...)
}

// insertOpen adds todos at the end of the open ones.
func insertOpen(items []*Item, added ...*Item) []*Item {
	open := 0
	for _, t := range items {
		if !t.Done() {
			open++
		}
	}
	list := append(slices.Clone(items[:open]), added...)
	return sortItems(append(list, items[open:]...))
}

func validateText(message, description string) error {
	if strings.TrimSpace(message) == "" {
		return newAPIError(http.StatusBadRequest, "a todo needs some text")
	}
	if utf8.RuneCountInString(message) > maxMessageLength {
		return newAPIError(http.StatusBadRequest, "a todo is at most %d characters", maxMessageLength)
	}
	if utf8.RuneCountInString(description) > maxDescription {
		return newAPIError(http.StatusBadRequest, "a description is at most %d characters", maxDescription)
	}
	return nil
}

func validateDue(due string) error {
	if due == "" {
		return nil
	}
	if !dueFormat.MatchString(due) {
		return newAPIError(http.StatusBadRequest, "a due date is YYYY-MM-DD")
	}
	if _, err := time.Parse(time.DateOnly, due); err != nil {
		return newAPIError(http.StatusBadRequest, "a due date is YYYY-MM-DD")
	}
	return nil
}

// checkAssignee: only a member of a channel's list's channel can be assigned its todos; personal
// lists' todos are sent instead (SendItem).
func (p *Plugin) checkAssignee(list *TodoList, assigneeID string) error {
	if assigneeID == "" {
		return nil
	}
	if list.ChannelID == "" {
		return newAPIError(http.StatusBadRequest, "send a todo of a personal list instead of assigning it")
	}
	if _, appErr := p.API.GetChannelMember(list.ChannelID, assigneeID); appErr != nil {
		return newAPIError(http.StatusBadRequest, "only a member of the channel can be assigned")
	}
	return nil
}

// AddItem adds a todo at the end of a list's open todos.
func (p *Plugin) AddItem(userID, listID string, req itemRequest) (*Item, error) {
	list, rights, err := p.readableList(userID, listID)
	if err != nil {
		return nil, err
	}
	if !rights.Edit {
		return nil, newAPIError(http.StatusForbidden, "you can't add todos to this list")
	}
	now := model.GetMillis()
	item := &Item{ID: model.NewId(), ListID: list.ID, CreatorID: userID, CreateAt: now, UpdateAt: now}
	if req.Message != nil {
		item.Message = strings.TrimSpace(*req.Message)
	}
	if req.Description != nil {
		item.Description = strings.TrimSpace(*req.Description)
	}
	if err = validateText(item.Message, item.Description); err != nil {
		return nil, err
	}
	if req.Due != nil {
		item.Due = *req.Due
	}
	if err = validateDue(item.Due); err != nil {
		return nil, err
	}
	if req.AssigneeID != nil {
		item.AssigneeID = *req.AssigneeID
	}
	if err = p.checkAssignee(list, item.AssigneeID); err != nil {
		return nil, err
	}
	replyInThread := false
	if req.PostID != "" {
		post, permalink, postErr := p.readablePost(userID, req.PostID)
		if postErr != nil {
			return nil, postErr
		}
		item.PostID, item.PostPermalink = post.Id, permalink
		// The message's thread isn't told about the list of another channel, which its readers
		// may not see.
		replyInThread = list.ChannelID == "" || post.ChannelId == list.ChannelID
	}

	if _, err = updateKV(p.API, itemsKey(list.ID), func(items *[]*Item) error {
		open, _ := summarize(*items)
		if open >= maxOpenItems {
			return newAPIError(http.StatusBadRequest, "this list already has %d open todos", maxOpenItems)
		}
		*items = insertOpen(*items, item)
		return nil
	}); err != nil {
		return nil, err
	}

	if item.AssigneeID != "" {
		p.assign(userID, list, rights, item, "")
	}
	p.rememberLastUsed(userID, req.ChannelID, list.ID)
	p.sendListChanged(list)
	if replyInThread {
		userName := p.listManager.GetUserName(userID)
		reply := fmt.Sprintf("@%s attached a todo to this thread", userName)
		if list.ChannelID != "" {
			reply = fmt.Sprintf("@%s added this to the todo list %s", userName, list.Name)
		}
		p.postReplyIfNeeded(item.PostID, reply, item.Message, item.PostPermalink)
	}
	return item, nil
}

// UpdateItem edits, assigns, dates, checks off, reopens or moves a todo.
func (p *Plugin) UpdateItem(userID, listID, itemID string, req itemRequest) (*Item, error) {
	list, rights, err := p.readableList(userID, listID)
	if err != nil {
		return nil, err
	}
	if !rights.Edit {
		return nil, newAPIError(http.StatusForbidden, "you can't change the todos of this list")
	}
	if req.AssigneeID != nil {
		if err = p.checkAssignee(list, *req.AssigneeID); err != nil {
			return nil, err
		}
	}
	if req.Due != nil {
		if err = validateDue(*req.Due); err != nil {
			return nil, err
		}
	}

	var updated, previous Item
	_, err = updateKV(p.API, itemsKey(list.ID), func(items *[]*Item) error {
		index := slices.IndexFunc(*items, func(t *Item) bool { return t.ID == itemID })
		if index < 0 {
			return newAPIError(http.StatusNotFound, "todo not found")
		}
		previous = *(*items)[index]
		todo := previous
		if req.Message != nil || req.Description != nil {
			message, description := todo.Message, todo.Description
			if req.Message != nil {
				message = strings.TrimSpace(*req.Message)
			}
			if req.Description != nil {
				description = strings.TrimSpace(*req.Description)
			}
			if invalid := validateText(message, description); invalid != nil {
				return invalid
			}
			todo.Message, todo.Description = message, description
		}
		if req.AssigneeID != nil {
			todo.AssigneeID = *req.AssigneeID
		}
		if req.Due != nil {
			todo.Due = *req.Due
		}
		if req.Done != nil {
			if *req.Done && !todo.Done() {
				todo.DoneAt, todo.DoneBy = model.GetMillis(), userID
				// Done, a todo someone sent leaves their "Sent by you" (below)
				todo.FromUserID, todo.FromIssueID = "", ""
			} else if !*req.Done {
				todo.DoneAt, todo.DoneBy = 0, ""
			}
		}
		todo.UpdateAt = model.GetMillis()

		rest := slices.Delete(slices.Clone(*items), index, index+1)
		position := index
		if req.Position != nil {
			position = *req.Position
		}
		if req.Done != nil && !*req.Done && req.Position == nil && previous.Done() {
			// A reopened todo goes back to the end of the open ones.
			position, _ = summarize(rest)
		}
		position = min(max(position, 0), len(rest))
		*items = sortItems(slices.Insert(rest, position, &todo))
		updated = todo
		return nil
	})
	if err != nil {
		return nil, err
	}

	if updated.AssigneeID != previous.AssigneeID || updated.Done() != previous.Done() {
		p.assign(userID, list, rights, &updated, previous.AssigneeID)
	}
	if previous.FromIssueID != "" {
		switch {
		case updated.Done() && !previous.Done():
			p.releaseRequest(userID, &previous, requestCompleted)
		case updated.Message != previous.Message || updated.Description != previous.Description:
			p.editedRequest(userID, &previous, updated.Message, updated.Description)
		}
	}
	p.sendListChanged(list)
	return &updated, nil
}

// DeleteItem deletes a todo: of a channel's list, its author or a channel admin may.
func (p *Plugin) DeleteItem(userID, listID, itemID string) error {
	list, rights, err := p.readableList(userID, listID)
	if err != nil {
		return err
	}
	var removed *Item
	if _, err = updateKV(p.API, itemsKey(list.ID), func(items *[]*Item) error {
		index := slices.IndexFunc(*items, func(t *Item) bool { return t.ID == itemID })
		if index < 0 {
			return newAPIError(http.StatusNotFound, "todo not found")
		}
		t := (*items)[index]
		if !rights.Edit || (t.CreatorID != userID && !rights.Manage) {
			return newAPIError(http.StatusForbidden, "only its author or a channel admin can delete this todo")
		}
		removed = t
		*items = slices.Delete(*items, index, index+1)
		return nil
	}); err != nil {
		return err
	}
	if removed.AssigneeID != "" {
		p.unindexAssigned(removed.AssigneeID, removed.ID)
	}
	if removed.FromIssueID != "" {
		p.releaseRequest(userID, removed, requestDeclined)
	}
	p.sendListChanged(list)
	return nil
}

// takeItem removes a todo from a list, without telling anyone: it's moving somewhere else.
func (p *Plugin) takeItem(listID, itemID string) (*Item, error) {
	var taken *Item
	_, err := updateKV(p.API, itemsKey(listID), func(items *[]*Item) error {
		index := slices.IndexFunc(*items, func(t *Item) bool { return t.ID == itemID })
		if index < 0 {
			return newAPIError(http.StatusNotFound, "todo not found")
		}
		taken = (*items)[index]
		*items = slices.Delete(*items, index, index+1)
		return nil
	})
	return taken, err
}

// Assignments -------------------------------------------------------------------------------------

// assign keeps the assignees' lists up to date and tells a new assignee, when someone else assigned
// them.
func (p *Plugin) assign(userID string, list *TodoList, rights listRights, todo *Item, previous string) {
	if previous != "" && (previous != todo.AssigneeID || todo.Done()) {
		p.unindexAssigned(previous, todo.ID)
	}
	if todo.AssigneeID == "" || todo.Done() || list.ChannelID == "" {
		return
	}
	// Listing is idempotent: a reopened todo is listed again for its assignee.
	if _, err := updateKV(p.API, assignedKey(todo.AssigneeID), func(refs *[]assignedRef) error {
		for _, r := range *refs {
			if r.TodoID == todo.ID {
				return nil
			}
		}
		*refs = append(*refs, assignedRef{ChannelID: list.ChannelID, ListID: list.ID, TodoID: todo.ID})
		return nil
	}); err != nil {
		p.API.LogWarn("Could not list an assigned todo", "error", err.Error())
	}
	p.sendRefreshEvent(todo.AssigneeID, []string{AssignedListKey})
	if todo.AssigneeID != userID && todo.AssigneeID != previous {
		where := fmt.Sprintf("%s (~%s)", list.Name, rights.Channel.Name)
		if isConversation(rights.Channel) {
			where = list.Name + " (a conversation)"
		}
		message := fmt.Sprintf("@%s assigned you a todo in %s:\n> %s", p.listManager.GetUserName(userID), where, todo.Message)
		if todo.Due != "" {
			message += fmt.Sprintf("\n> Due %s", todo.Due)
		}
		if todo.PostPermalink != "" {
			message += fmt.Sprintf("\n> [Permalink](%s)", todo.PostPermalink)
		}
		p.PostBotDM(todo.AssigneeID, message)
	}
}

func (p *Plugin) unindexAssigned(userID, todoID string) {
	if _, err := updateKV(p.API, assignedKey(userID), func(refs *[]assignedRef) error {
		*refs = slices.DeleteFunc(*refs, func(r assignedRef) bool { return r.TodoID == todoID })
		return nil
	}); err != nil {
		p.API.LogWarn("Could not unlist an assigned todo", "error", err.Error())
	}
	p.sendRefreshEvent(userID, []string{AssignedListKey})
}

// AssignedTodos returns the open todos assigned to someone, in the lists they can still read; the
// others drop out of their list.
func (p *Plugin) AssignedTodos(userID string) ([]*AssignedTodo, error) {
	data, appErr := p.API.KVGet(assignedKey(userID))
	if appErr != nil {
		return nil, appErr
	}
	var refs []assignedRef
	if data != nil {
		if err := json.Unmarshal(data, &refs); err != nil {
			return nil, err
		}
	}
	result := []*AssignedTodo{}
	items := map[string][]*Item{}
	stale := false
	for _, ref := range refs {
		if err := p.migrateChannel(ref.ChannelID); err != nil {
			return nil, err
		}
		list, err := p.getList(ref.listID())
		if err != nil {
			return nil, err
		}
		if list == nil {
			stale = true
			continue
		}
		rights, err := p.listRights(userID, list)
		if err != nil {
			return nil, err
		}
		if !rights.Read || list.ArchivedAt != 0 {
			stale = stale || !rights.Read
			continue
		}
		todos, ok := items[list.ID]
		if !ok {
			if todos, err = p.getItems(list.ID); err != nil {
				return nil, err
			}
			items[list.ID] = todos
		}
		i := slices.IndexFunc(todos, func(t *Item) bool { return t.ID == ref.TodoID })
		if i < 0 || todos[i].Done() || todos[i].AssigneeID != userID {
			stale = true
			continue
		}
		info := p.listInfo(userID, list, rights, nil, nil)
		result = append(result, &AssignedTodo{
			Item:           *todos[i],
			ChannelID:      list.ChannelID,
			ListName:       list.Name,
			ListIcon:       list.Icon,
			ChannelName:    info.ChannelName,
			ChannelURLName: info.ChannelURLName,
			TeamName:       info.TeamName,
		})
	}
	if stale {
		p.pruneAssigned(userID, result, refs)
	}
	return result, nil
}

// pruneAssigned forgets the assignments that aren't anymore, keeping those of archived lists, which
// come back when they're unarchived.
func (p *Plugin) pruneAssigned(userID string, kept []*AssignedTodo, refs []assignedRef) {
	keep := map[string]bool{}
	for _, item := range kept {
		keep[item.ID] = true
	}
	for _, ref := range refs {
		if list, err := p.getList(ref.listID()); err == nil && list != nil && list.ArchivedAt != 0 {
			if rights, err := p.listRights(userID, list); err == nil && rights.Read {
				keep[ref.TodoID] = true
			}
		}
	}
	if _, err := updateKV(p.API, assignedKey(userID), func(refs *[]assignedRef) error {
		*refs = slices.DeleteFunc(*refs, func(r assignedRef) bool { return !keep[r.TodoID] })
		return nil
	}); err != nil {
		p.API.LogWarn("Could not prune the assigned todos", "error", err.Error())
	}
}
