// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/pkg/errors"
)

// A channel's todo list is shared by its members: everyone who can post in the channel adds,
// checks off, edits and assigns its todos; their creator and the channel's managers delete them.
// Done todos stay listed, under the open ones.

const (
	// WSEventChannelRefresh tells a channel's members that its todo list changed.
	WSEventChannelRefresh = "channel_refresh"
	// AssignedListKey is the name of the refreshed list when someone's channel assignments change.
	AssignedListKey = "assigned"

	storeChannelListKey = "chlist"
	storeAssignedKey    = "chassigned"

	maxOpenChannelTodos = 500
	maxDoneChannelTodos = 200
	maxMessageLength    = 1000
	maxDescription      = 4000
)

// ChannelTodo is a todo of a channel's list.
type ChannelTodo struct {
	ID            string `json:"id"`
	ChannelID     string `json:"channel_id"`
	Message       string `json:"message"`
	Description   string `json:"description,omitempty"`
	CreatorID     string `json:"creator_id"`
	CreateAt      int64  `json:"create_at"`
	UpdateAt      int64  `json:"update_at"`
	AssigneeID    string `json:"assignee_id,omitempty"`
	DoneBy        string `json:"done_by,omitempty"`
	DoneAt        int64  `json:"done_at,omitempty"`
	PostID        string `json:"post_id,omitempty"`
	PostPermalink string `json:"post_permalink,omitempty"`
}

// Done tells whether the todo is checked off.
func (t *ChannelTodo) Done() bool { return t.DoneAt != 0 }

// assignedRef points to a channel todo assigned to someone.
type assignedRef struct {
	ChannelID string `json:"channel_id"`
	TodoID    string `json:"todo_id"`
}

// AssignedTodo is a channel todo assigned to the person asking, with where it is.
type AssignedTodo struct {
	ChannelTodo
	ChannelName string `json:"channel_name"`
	TeamName    string `json:"team_name,omitempty"`
}

// ChannelPermissions say what the person asking may do with a channel's list.
type ChannelPermissions struct {
	CanEdit   bool `json:"can_edit"`
	CanManage bool `json:"can_manage"`
}

// ChannelListResponse is a channel's list, open todos first, then the done ones, newest first.
type ChannelListResponse struct {
	ChannelPermissions
	Todos []*ChannelTodo `json:"todos"`
}

type channelTodoRequest struct {
	Message     *string `json:"message"`
	Description *string `json:"description"`
	AssigneeID  *string `json:"assignee_id"`
	Done        *bool   `json:"done"`
	// Position moves an open todo to that index among the open ones.
	Position *int   `json:"position"`
	PostID   string `json:"post_id"`
}

type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func newAPIError(status int, format string, args ...any) *apiError {
	return &apiError{status: status, msg: fmt.Sprintf(format, args...)}
}

func channelListKey(channelID string) string { return storeChannelListKey + "_" + channelID }
func assignedKey(userID string) string       { return storeAssignedKey + "_" + userID }

// updateKV changes a JSON value of the KV store atomically: it reads it, applies change, and writes
// it back if nobody changed it meanwhile (retrying otherwise).
func updateKV[T any](p *Plugin, key string, change func(*T) error) (*T, error) {
	for range StoreRetries + 2 {
		old, appErr := p.API.KVGet(key)
		if appErr != nil {
			return nil, appErr
		}
		var value T
		if old != nil {
			if err := json.Unmarshal(old, &value); err != nil {
				return nil, err
			}
		}
		if err := change(&value); err != nil {
			return nil, err
		}
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(data, old) {
			return &value, nil
		}
		ok, appErr := p.API.KVCompareAndSet(key, old, data)
		if appErr != nil {
			return nil, appErr
		}
		if ok {
			return &value, nil
		}
	}
	return nil, errors.New("the list changed too often, try again")
}

func (p *Plugin) getChannelTodos(channelID string) ([]*ChannelTodo, error) {
	data, appErr := p.API.KVGet(channelListKey(channelID))
	if appErr != nil {
		return nil, appErr
	}
	todos := []*ChannelTodo{}
	if data != nil {
		if err := json.Unmarshal(data, &todos); err != nil {
			return nil, err
		}
	}
	return todos, nil
}

// sortChannelTodos keeps the open todos in their order, then the done ones, newest first, keeping at
// most maxDoneChannelTodos of them.
func sortChannelTodos(todos []*ChannelTodo) []*ChannelTodo {
	open := make([]*ChannelTodo, 0, len(todos))
	var done []*ChannelTodo
	for _, t := range todos {
		if t.Done() {
			done = append(done, t)
		} else {
			open = append(open, t)
		}
	}
	// Insertion sort: the done todos are already almost in order.
	for i := 1; i < len(done); i++ {
		for j := i; j > 0 && done[j].DoneAt > done[j-1].DoneAt; j-- {
			done[j], done[j-1] = done[j-1], done[j]
		}
	}
	if len(done) > maxDoneChannelTodos {
		done = done[:maxDoneChannelTodos]
	}
	return append(open, done...)
}

func (p *Plugin) channelPermissions(userID string, channel *model.Channel) ChannelPermissions {
	canEdit := channel.DeleteAt == 0 && p.API.HasPermissionToChannel(userID, channel.Id, model.PermissionCreatePost)
	return ChannelPermissions{CanEdit: canEdit, CanManage: canEdit && p.canManageChannel(userID, channel)}
}

func (p *Plugin) canManageChannel(userID string, channel *model.Channel) bool {
	switch channel.Type {
	case model.ChannelTypeOpen:
		return p.API.HasPermissionToChannel(userID, channel.Id, model.PermissionManagePublicChannelProperties)
	case model.ChannelTypePrivate:
		return p.API.HasPermissionToChannel(userID, channel.Id, model.PermissionManagePrivateChannelProperties)
	default:
		// Nobody manages direct and group messages: their todos are deleted by their creators.
		return false
	}
}

// readableChannel returns a channel the person may read.
func (p *Plugin) readableChannel(userID, channelID string) (*model.Channel, error) {
	channel, appErr := p.API.GetChannel(channelID)
	if appErr != nil {
		return nil, newAPIError(http.StatusNotFound, "channel not found")
	}
	if !p.API.HasPermissionToChannel(userID, channelID, model.PermissionReadChannel) {
		return nil, newAPIError(http.StatusForbidden, "you can't see this channel")
	}
	return channel, nil
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

func (p *Plugin) checkAssignee(channelID, assigneeID string) error {
	if assigneeID == "" {
		return nil
	}
	if _, appErr := p.API.GetChannelMember(channelID, assigneeID); appErr != nil {
		return newAPIError(http.StatusBadRequest, "only a member of the channel can be assigned")
	}
	return nil
}

// AddChannelTodo adds a todo at the end of a channel's open todos.
func (p *Plugin) AddChannelTodo(userID, channelID string, req channelTodoRequest) (*ChannelTodo, error) {
	channel, err := p.readableChannel(userID, channelID)
	if err != nil {
		return nil, err
	}
	if !p.channelPermissions(userID, channel).CanEdit {
		return nil, newAPIError(http.StatusForbidden, "you can't add todos to this channel")
	}
	todo := &ChannelTodo{ID: model.NewId(), ChannelID: channelID, CreatorID: userID, CreateAt: model.GetMillis()}
	todo.UpdateAt = todo.CreateAt
	if req.Message != nil {
		todo.Message = strings.TrimSpace(*req.Message)
	}
	if req.Description != nil {
		todo.Description = strings.TrimSpace(*req.Description)
	}
	if err = validateText(todo.Message, todo.Description); err != nil {
		return nil, err
	}
	if req.AssigneeID != nil {
		todo.AssigneeID = *req.AssigneeID
	}
	if err = p.checkAssignee(channelID, todo.AssigneeID); err != nil {
		return nil, err
	}
	if req.PostID != "" {
		post, appErr := p.API.GetPost(req.PostID)
		if appErr != nil || post.ChannelId != channelID {
			return nil, newAPIError(http.StatusBadRequest, "the message isn't in this channel")
		}
		todo.PostID = post.Id
		todo.PostPermalink = p.permalink(channel, post.Id)
	}

	_, err = updateKV(p, channelListKey(channelID), func(todos *[]*ChannelTodo) error {
		open := 0
		for _, t := range *todos {
			if !t.Done() {
				open++
			}
		}
		if open >= maxOpenChannelTodos {
			return newAPIError(http.StatusBadRequest, "this channel already has %d open todos", maxOpenChannelTodos)
		}
		insertAt := open
		list := append([]*ChannelTodo{}, (*todos)[:insertAt]...)
		list = append(list, todo)
		*todos = sortChannelTodos(append(list, (*todos)[insertAt:]...))
		return nil
	})
	if err != nil {
		return nil, err
	}

	if todo.AssigneeID != "" {
		p.assign(userID, channel, todo, "")
	}
	p.sendChannelRefresh(channelID)
	if todo.PostID != "" {
		p.postReplyIfNeeded(todo.PostID, fmt.Sprintf("@%s added this to the todo list of the channel", p.listManager.GetUserName(userID)), todo.Message, todo.PostPermalink)
	}
	return todo, nil
}

// UpdateChannelTodo edits, assigns, checks off, reopens or moves a channel's todo.
func (p *Plugin) UpdateChannelTodo(userID, channelID, todoID string, req channelTodoRequest) (*ChannelTodo, error) {
	channel, err := p.readableChannel(userID, channelID)
	if err != nil {
		return nil, err
	}
	if !p.channelPermissions(userID, channel).CanEdit {
		return nil, newAPIError(http.StatusForbidden, "you can't change the todos of this channel")
	}
	if req.AssigneeID != nil {
		if err = p.checkAssignee(channelID, *req.AssigneeID); err != nil {
			return nil, err
		}
	}

	var updated ChannelTodo
	var previousAssignee string
	_, err = updateKV(p, channelListKey(channelID), func(todos *[]*ChannelTodo) error {
		index := -1
		for i, t := range *todos {
			if t.ID == todoID {
				index = i
				break
			}
		}
		if index < 0 {
			return newAPIError(http.StatusNotFound, "todo not found")
		}
		todo := *(*todos)[index]
		previousAssignee = todo.AssigneeID
		if req.Message != nil || req.Description != nil {
			message, description := todo.Message, todo.Description
			if req.Message != nil {
				message = strings.TrimSpace(*req.Message)
			}
			if req.Description != nil {
				description = strings.TrimSpace(*req.Description)
			}
			if err := validateText(message, description); err != nil {
				return err
			}
			todo.Message, todo.Description = message, description
		}
		if req.AssigneeID != nil {
			todo.AssigneeID = *req.AssigneeID
		}
		if req.Done != nil {
			if *req.Done && !todo.Done() {
				todo.DoneAt, todo.DoneBy = model.GetMillis(), userID
			} else if !*req.Done {
				todo.DoneAt, todo.DoneBy = 0, ""
			}
		}
		todo.UpdateAt = model.GetMillis()

		list := append(append([]*ChannelTodo{}, (*todos)[:index]...), (*todos)[index+1:]...)
		position := index
		if req.Position != nil {
			position = *req.Position
		}
		if req.Done != nil && !*req.Done && req.Position == nil {
			// A reopened todo goes back to the end of the open ones.
			position = len(list)
		}
		if position < 0 {
			position = 0
		}
		if position > len(list) {
			position = len(list)
		}
		list = append(list[:position], append([]*ChannelTodo{&todo}, list[position:]...)...)
		*todos = sortChannelTodos(list)
		updated = todo
		return nil
	})
	if err != nil {
		return nil, err
	}

	if updated.AssigneeID != previousAssignee || req.Done != nil {
		p.assign(userID, channel, &updated, previousAssignee)
	}
	p.sendChannelRefresh(channelID)
	return &updated, nil
}

// DeleteChannelTodo deletes a channel's todo: its creator or a manager of the channel may.
func (p *Plugin) DeleteChannelTodo(userID, channelID, todoID string) error {
	channel, err := p.readableChannel(userID, channelID)
	if err != nil {
		return err
	}
	perms := p.channelPermissions(userID, channel)
	var removed *ChannelTodo
	_, err = updateKV(p, channelListKey(channelID), func(todos *[]*ChannelTodo) error {
		for i, t := range *todos {
			if t.ID != todoID {
				continue
			}
			if !perms.CanEdit || (t.CreatorID != userID && !perms.CanManage) {
				return newAPIError(http.StatusForbidden, "only its author or a channel admin can delete this todo")
			}
			removed = t
			*todos = append((*todos)[:i], (*todos)[i+1:]...)
			return nil
		}
		return newAPIError(http.StatusNotFound, "todo not found")
	})
	if err != nil {
		return err
	}
	if removed.AssigneeID != "" {
		p.unindexAssigned(removed.AssigneeID, channelID, todoID)
	}
	p.sendChannelRefresh(channelID)
	return nil
}

// assign keeps the assignees' lists up to date and tells a new assignee, when someone else assigned
// them.
func (p *Plugin) assign(userID string, channel *model.Channel, todo *ChannelTodo, previous string) {
	if previous != "" && (previous != todo.AssigneeID || todo.Done()) {
		p.unindexAssigned(previous, channel.Id, todo.ID)
	}
	if todo.AssigneeID == "" || todo.Done() {
		return
	}
	// Listing is idempotent: a reopened todo is listed again for its assignee.
	if _, err := updateKV(p, assignedKey(todo.AssigneeID), func(refs *[]assignedRef) error {
		for _, r := range *refs {
			if r.TodoID == todo.ID {
				return nil
			}
		}
		*refs = append(*refs, assignedRef{ChannelID: channel.Id, TodoID: todo.ID})
		return nil
	}); err != nil {
		p.API.LogWarn("Could not list an assigned todo", "error", err.Error())
	}
	p.sendRefreshEvent(todo.AssigneeID, []string{AssignedListKey})
	if todo.AssigneeID != userID && todo.AssigneeID != previous {
		where := "~" + channel.Name
		if channel.Type == model.ChannelTypeDirect || channel.Type == model.ChannelTypeGroup {
			where = "a conversation"
		}
		message := fmt.Sprintf("@%s assigned you a todo in %s:\n> %s", p.listManager.GetUserName(userID), where, todo.Message)
		if todo.PostPermalink != "" {
			message += fmt.Sprintf("\n> [Permalink](%s)", todo.PostPermalink)
		}
		p.PostBotDM(todo.AssigneeID, message)
	}
}

func (p *Plugin) unindexAssigned(userID, channelID, todoID string) {
	if _, err := updateKV(p, assignedKey(userID), func(refs *[]assignedRef) error {
		kept := (*refs)[:0]
		for _, r := range *refs {
			if !(r.ChannelID == channelID && r.TodoID == todoID) {
				kept = append(kept, r)
			}
		}
		*refs = kept
		return nil
	}); err != nil {
		p.API.LogWarn("Could not unlist an assigned todo", "error", err.Error())
	}
	p.sendRefreshEvent(userID, []string{AssignedListKey})
}

// AssignedTodos returns the open channel todos assigned to someone, in the channels they can still
// read; the others drop out of their list.
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
	lists := map[string][]*ChannelTodo{}
	stale := false
	for _, ref := range refs {
		channel, err := p.readableChannel(userID, ref.ChannelID)
		if err != nil {
			stale = true
			continue
		}
		todos, ok := lists[ref.ChannelID]
		if !ok {
			if todos, err = p.getChannelTodos(ref.ChannelID); err != nil {
				return nil, err
			}
			lists[ref.ChannelID] = todos
		}
		var found *ChannelTodo
		for _, t := range todos {
			if t.ID == ref.TodoID {
				found = t
				break
			}
		}
		if found == nil || found.Done() || found.AssigneeID != userID {
			stale = true
			continue
		}
		item := &AssignedTodo{ChannelTodo: *found, ChannelName: channel.DisplayName}
		if channel.TeamId != "" {
			if team, appErr := p.API.GetTeam(channel.TeamId); appErr == nil {
				item.TeamName = team.Name
			}
		}
		result = append(result, item)
	}
	if stale {
		keep := map[string]bool{}
		for _, item := range result {
			keep[item.ID] = true
		}
		if _, err := updateKV(p, assignedKey(userID), func(refs *[]assignedRef) error {
			kept := (*refs)[:0]
			for _, r := range *refs {
				if keep[r.TodoID] {
					kept = append(kept, r)
				}
			}
			*refs = kept
			return nil
		}); err != nil {
			p.API.LogWarn("Could not prune the assigned todos", "error", err.Error())
		}
	}
	return result, nil
}

func (p *Plugin) permalink(channel *model.Channel, postID string) string {
	siteURL := ""
	if s := p.API.GetConfig().ServiceSettings.SiteURL; s != nil {
		siteURL = strings.TrimSuffix(*s, "/")
	}
	if channel.TeamId != "" {
		if team, appErr := p.API.GetTeam(channel.TeamId); appErr == nil {
			return fmt.Sprintf("%s/%s/pl/%s", siteURL, team.Name, postID)
		}
	}
	return fmt.Sprintf("%s/_redirect/pl/%s", siteURL, postID)
}

func (p *Plugin) sendChannelRefresh(channelID string) {
	p.API.PublishWebSocketEvent(WSEventChannelRefresh, map[string]any{"channel_id": channelID}, &model.WebsocketBroadcast{ChannelId: channelID})
}

// HTTP --------------------------------------------------------------------------------------------

func (p *Plugin) initChannelAPI(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/channels/{channel_id:[a-z0-9]{26}}/todos", p.checkAuth(p.handleChannelTodos)).Methods(http.MethodGet)
	api.HandleFunc("/channels/{channel_id:[a-z0-9]{26}}/todos", p.checkAuth(p.handleAddChannelTodo)).Methods(http.MethodPost)
	api.HandleFunc("/channels/{channel_id:[a-z0-9]{26}}/todos/{todo_id:[a-z0-9]{26}}", p.checkAuth(p.handleUpdateChannelTodo)).Methods(http.MethodPatch)
	api.HandleFunc("/channels/{channel_id:[a-z0-9]{26}}/todos/{todo_id:[a-z0-9]{26}}", p.checkAuth(p.handleDeleteChannelTodo)).Methods(http.MethodDelete)
	api.HandleFunc("/assigned", p.checkAuth(p.handleAssigned)).Methods(http.MethodGet)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var a *apiError
	var appErr *model.AppError
	switch {
	case errors.As(err, &a):
		status = a.status
	case errors.As(err, &appErr) && appErr.StatusCode != 0:
		status = appErr.StatusCode
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decodeRequest(w http.ResponseWriter, r *http.Request) (channelTodoRequest, error) {
	var req channelTodoRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		return req, newAPIError(http.StatusBadRequest, "invalid request")
	}
	return req, nil
}

func (p *Plugin) handleChannelTodos(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	channelID := mux.Vars(r)["channel_id"]
	channel, err := p.readableChannel(userID, channelID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	todos, err := p.getChannelTodos(channelID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ChannelListResponse{ChannelPermissions: p.channelPermissions(userID, channel), Todos: todos})
}

func (p *Plugin) handleAddChannelTodo(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRequest(w, r)
	if err == nil {
		var todo *ChannelTodo
		if todo, err = p.AddChannelTodo(r.Header.Get("Mattermost-User-ID"), mux.Vars(r)["channel_id"], req); err == nil {
			writeJSON(w, http.StatusCreated, todo)
			return
		}
	}
	writeAPIError(w, err)
}

func (p *Plugin) handleUpdateChannelTodo(w http.ResponseWriter, r *http.Request) {
	req, err := decodeRequest(w, r)
	if err == nil {
		var todo *ChannelTodo
		vars := mux.Vars(r)
		if todo, err = p.UpdateChannelTodo(r.Header.Get("Mattermost-User-ID"), vars["channel_id"], vars["todo_id"], req); err == nil {
			writeJSON(w, http.StatusOK, todo)
			return
		}
	}
	writeAPIError(w, err)
}

func (p *Plugin) handleDeleteChannelTodo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if err := p.DeleteChannelTodo(r.Header.Get("Mattermost-User-ID"), vars["channel_id"], vars["todo_id"]); err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}

func (p *Plugin) handleAssigned(w http.ResponseWriter, r *http.Request) {
	todos, err := p.AssignedTodos(r.Header.Get("Mattermost-User-ID"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, todos)
}
