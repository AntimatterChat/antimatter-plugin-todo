// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// Todos sent to someone ("requests") work as before lists: the sender keeps a copy in their "Sent
// by you", the receiver gets theirs in "Sent to you" (list.go, store.go), and answers it. Accepted,
// it becomes a todo of one of the receiver's personal lists (their default one unless they pick
// another), linked to the sender's copy: done or deleted, the sender is told and their copy goes;
// edited by the sender, it changes in the list; cancelled by the sender, it leaves the list.
//
// The receiver's copy (item_<id>) stays while the todo is in their list, with the list's ID, so
// that the sender's side finds it.

const (
	requestCompleted = "completed"
	requestDeclined  = "declined"
)

// SendItem sends a todo of a personal list to someone: it leaves the list, and they get it as a
// request. Only members of a channel's list's channel are assigned its todos instead.
func (p *Plugin) SendItem(userID, listID, itemID, username string) (string, error) {
	list, rights, err := p.readableList(userID, listID)
	if err != nil {
		return "", err
	}
	if list.ChannelID != "" {
		return "", newAPIError(http.StatusBadRequest, "assign the todos of a channel's list instead")
	}
	if !rights.Edit {
		return "", newAPIError(http.StatusForbidden, "you can't change the todos of this list")
	}
	items, err := p.getItems(list.ID)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(items, func(t *Item) bool { return t.ID == itemID })
	if i < 0 {
		return "", newAPIError(http.StatusNotFound, "todo not found")
	}
	item := items[i]
	if item.FromIssueID != "" {
		return "", newAPIError(http.StatusBadRequest, "a todo someone sent you can't be sent on")
	}
	if item.Done() {
		return "", newAPIError(http.StatusBadRequest, "this todo is done already")
	}
	receiver, appErr := p.API.GetUserByUsername(strings.TrimPrefix(username, "@"))
	if appErr != nil {
		return "", newAPIError(http.StatusNotFound, "user not found")
	}
	if receiver.Id == userID {
		return "", newAPIError(http.StatusBadRequest, "this todo is yours already")
	}
	if allowed, err := p.getAllowIncomingTaskRequestsPreference(receiver.Id); err == nil && !allowed {
		return "", newAPIError(http.StatusForbidden, "@%s has blocked Todo requests", receiver.Username)
	}

	receiverIssueID, err := p.listManager.SendIssue(userID, receiver.Id, item.Message, item.PostPermalink, item.Description, item.PostID)
	if err != nil {
		return "", err
	}
	if _, err = p.takeItem(list.ID, item.ID); err != nil {
		// Sent, and still in the list: twice rather than lost.
		p.API.LogWarn("Could not remove a sent todo from its list", "error", err.Error())
	}
	p.sendListChanged(list)
	p.sendRefreshEvent(userID, []string{OutListKey})
	p.sendRefreshEvent(receiver.Id, []string{InListKey})
	senderName := p.listManager.GetUserName(userID)
	p.PostBotCustomDM(receiver.Id, fmt.Sprintf("You have received a new Todo from @%s", senderName), item.Message, item.PostPermalink, receiverIssueID)
	return receiver.Id, nil
}

// acceptRequest moves a todo someone sent into one of the receiver's personal lists (their default
// one when listID is empty), and returns its text and its sender.
func (p *Plugin) acceptRequest(userID, issueID, listID string) (string, string, error) {
	issue, ref, err := p.listManager.GetRequest(userID, issueID)
	if err != nil {
		return "", "", newAPIError(http.StatusNotFound, "request not found")
	}
	var list *TodoList
	if listID == "" {
		if list, err = p.ensurePersonal(userID); err != nil {
			return "", "", err
		}
	} else {
		var rights listRights
		if list, rights, err = p.readableList(userID, listID); err != nil {
			return "", "", err
		}
		if list.ChannelID != "" || !rights.Edit {
			return "", "", newAPIError(http.StatusBadRequest, "accept a todo into one of your own lists")
		}
	}

	now := model.GetMillis()
	item := &Item{
		ID:            issue.ID,
		ListID:        list.ID,
		Message:       issue.Message,
		Description:   issue.Description,
		CreatorID:     ref.ForeignUserID,
		CreateAt:      now,
		UpdateAt:      now,
		PostID:        issue.PostID,
		PostPermalink: issue.PostPermalink,
		FromUserID:    ref.ForeignUserID,
		FromIssueID:   ref.ForeignIssueID,
	}
	// Into the list first, out of the requests then: accepted twice (the second one finds it in
	// the list) rather than lost.
	if _, err = updateKV(p.API, itemsKey(list.ID), func(items *[]*Item) error {
		if !slices.ContainsFunc(*items, func(t *Item) bool { return t.ID == item.ID }) {
			*items = insertOpen(*items, item)
		}
		return nil
	}); err != nil {
		return "", "", err
	}
	if err = p.listManager.LinkRequest(userID, issueID, list.ID); err != nil {
		return "", "", err
	}
	p.sendListChanged(list)

	message := issue.Message
	if issue.PostPermalink != "" {
		message = fmt.Sprintf("%s\n[Permalink](%s)", message, issue.PostPermalink)
	}
	return message, ref.ForeignUserID, nil
}

// releaseRequest tells the sender of an accepted todo that it's done or was declined, and removes
// their copy.
func (p *Plugin) releaseRequest(userID string, item *Item, how string) {
	if err := p.listManager.ReleaseRequest(item.ID, item.FromUserID, item.FromIssueID); err != nil {
		p.API.LogWarn("Could not remove the sender's copy of a todo", "error", err.Error())
	}
	p.sendRefreshEvent(item.FromUserID, []string{OutListKey})
	message := item.Message
	if item.PostPermalink != "" {
		message = fmt.Sprintf("%s\n[Permalink](%s)", message, item.PostPermalink)
	}
	p.PostBotDM(item.FromUserID, fmt.Sprintf("@%s %s a Todo you sent: %s", p.listManager.GetUserName(userID), how, message))
}

// editedRequest changes the sender's copy of an accepted todo its receiver edited, and tells them.
func (p *Plugin) editedRequest(userID string, item *Item, message, description string) {
	old, err := p.listManager.UpdateRequestText(item.FromIssueID, item.ID, message, description)
	if err != nil {
		p.API.LogWarn("Could not edit the sender's copy of a todo", "error", err.Error())
		return
	}
	p.sendRefreshEvent(item.FromUserID, []string{OutListKey})
	p.PostBotDM(item.FromUserID, fmt.Sprintf("@%s modified a Todo from:\n%s\nTo:\n%s", p.listManager.GetUserName(userID), old, message))
}

// RemoveLinkedItem takes an accepted todo out of its list, when its sender cancels it or sends it
// to someone else (LinkedItems).
func (p *Plugin) RemoveLinkedItem(listID, itemID string) {
	if _, err := p.takeItem(listID, itemID); err != nil {
		p.API.LogWarn("Could not remove a cancelled todo from its list", "error", err.Error())
		return
	}
	if list, err := p.getList(listID); err == nil && list != nil {
		p.sendListChanged(list)
	}
}

// EditLinkedItem changes the text of an accepted todo its sender edited (LinkedItems).
func (p *Plugin) EditLinkedItem(listID, itemID, message, description string) {
	if _, err := updateKV(p.API, itemsKey(listID), func(items *[]*Item) error {
		for _, t := range *items {
			if t.ID == itemID {
				t.Message, t.Description, t.UpdateAt = message, description, model.GetMillis()
			}
		}
		return nil
	}); err != nil {
		p.API.LogWarn("Could not edit an accepted todo", "error", err.Error())
		return
	}
	if list, err := p.getList(listID); err == nil && list != nil {
		p.sendListChanged(list)
	}
}
