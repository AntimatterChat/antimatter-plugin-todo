// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendToBob sends a todo of alice's default list to bob, and returns bob's copy, waiting for his
// answer.
func sendToBob(t *testing.T, p *Plugin, message string) *ExtendedIssue {
	t.Helper()
	mine, err := p.ensurePersonal(alice)
	require.NoError(t, err)
	item := addItem(t, p, alice, mine.ID, message)
	_, err = p.SendItem(alice, mine.ID, item.ID, "@bob")
	require.NoError(t, err)
	received, err := p.listManager.GetIssueList(bob, InListKey)
	require.NoError(t, err)
	require.NotEmpty(t, received)
	return received[len(received)-1]
}

func sent(t *testing.T, p *Plugin, userID string) []*ExtendedIssue {
	t.Helper()
	issues, err := p.listManager.GetIssueList(userID, OutListKey)
	require.NoError(t, err)
	return issues
}

func TestSendingATodoOfAPersonalList(t *testing.T) {
	p, api := newTestPlugin(t)
	request := sendToBob(t, p, "Sign the PO")
	assert.Empty(t, messages(t, p, defaultPersonalListID(alice)), "it leaves the sender's list")
	assert.Equal(t, "Sign the PO", request.Message)
	require.Len(t, sent(t, p, alice), 1, "for their Sent by you")
	require.Len(t, api.dms[bob], 1)
	assert.Contains(t, api.dms[bob][0], "You have received a new Todo from @alice")

	launch := newList(t, p, alice, town, "Launch")
	item := addItem(t, p, alice, launch.ID, "x")
	_, err := p.SendItem(alice, launch.ID, item.ID, "bob")
	assert.Equal(t, http.StatusBadRequest, status(err), "a channel's todos are assigned instead")
	groceries := newList(t, p, alice, "", "Groceries")
	milk := addItem(t, p, alice, groceries.ID, "Oat milk")
	_, err = p.SendItem(alice, groceries.ID, milk.ID, "alice")
	assert.Equal(t, http.StatusBadRequest, status(err))
	_, err = p.SendItem(alice, groceries.ID, milk.ID, "nobody")
	assert.Equal(t, http.StatusNotFound, status(err))
	_, err = p.SendItem(bob, groceries.ID, milk.ID, "carol")
	assert.Equal(t, http.StatusNotFound, status(err), "nor someone else's")

	require.NoError(t, p.saveAllowIncomingTaskRequestsPreference(carol, false))
	_, err = p.SendItem(alice, groceries.ID, milk.ID, "carol")
	assert.Equal(t, http.StatusForbidden, status(err), "carol takes no requests")
	assert.Equal(t, []string{"Oat milk"}, messages(t, p, groceries.ID))
}

func TestAcceptedTodosGoToAListAndTellTheSenderWhenDone(t *testing.T) {
	p, api := newTestPlugin(t)
	request := sendToBob(t, p, "Sign the PO")

	message, sender, err := p.acceptRequest(bob, request.ID, "")
	require.NoError(t, err)
	assert.Equal(t, "Sign the PO", message)
	assert.Equal(t, alice, sender)
	items, err := p.getItems(defaultPersonalListID(bob))
	require.NoError(t, err)
	require.Len(t, items, 1, "into bob's default list")
	assert.Equal(t, request.ID, items[0].ID)
	assert.Equal(t, alice, items[0].FromUserID)
	waiting, err := p.listManager.GetIssueList(bob, InListKey)
	require.NoError(t, err)
	assert.Empty(t, waiting, "no longer waiting")
	assert.Len(t, sent(t, p, alice), 1, "still in alice's Sent by you")

	_, _, err = p.acceptRequest(bob, request.ID, "")
	assert.Equal(t, http.StatusNotFound, status(err), "accepted once")

	_, err = p.UpdateItem(bob, defaultPersonalListID(bob), request.ID, itemRequest{Message: str("Sign the PO today")})
	require.NoError(t, err)
	out := sent(t, p, alice)
	require.Len(t, out, 1)
	assert.Equal(t, "Sign the PO today", out[0].Message, "the sender's copy follows edits")
	assert.Contains(t, api.dms[alice][len(api.dms[alice])-1], "@bob modified a Todo")

	_, err = p.UpdateItem(bob, defaultPersonalListID(bob), request.ID, itemRequest{Done: yes()})
	require.NoError(t, err)
	assert.Empty(t, sent(t, p, alice), "done: it leaves the sender's list")
	assert.Contains(t, api.dms[alice][len(api.dms[alice])-1], "@bob completed a Todo you sent: Sign the PO today")
	items, err = p.getItems(defaultPersonalListID(bob))
	require.NoError(t, err)
	assert.True(t, items[0].Done(), "and stays done in bob's list")
	assert.Empty(t, items[0].FromIssueID)
	told := len(api.dms[alice])
	_, err = p.UpdateItem(bob, defaultPersonalListID(bob), request.ID, itemRequest{Done: no()})
	require.NoError(t, err)
	_, err = p.UpdateItem(bob, defaultPersonalListID(bob), request.ID, itemRequest{Done: yes()})
	require.NoError(t, err)
	assert.Len(t, api.dms[alice], told, "once")
}

func TestAcceptingIntoAChosenList(t *testing.T) {
	p, _ := newTestPlugin(t)
	request := sendToBob(t, p, "Sign the PO")
	work := newList(t, p, bob, "", "Work")
	launch := newList(t, p, bob, town, "Launch")
	groceries := newList(t, p, alice, "", "Groceries")

	_, _, err := p.acceptRequest(bob, request.ID, launch.ID)
	assert.Equal(t, http.StatusBadRequest, status(err), "not into a channel's list, which others see")
	_, _, err = p.acceptRequest(bob, request.ID, groceries.ID)
	assert.Equal(t, http.StatusNotFound, status(err), "nor into someone else's")
	_, _, err = p.acceptRequest(carol, request.ID, "")
	assert.Equal(t, http.StatusNotFound, status(err), "nor someone else's request")
	_, _, err = p.acceptRequest(bob, request.ID, work.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Sign the PO"}, messages(t, p, work.ID))
}

func TestDecliningAnAcceptedTodo(t *testing.T) {
	p, api := newTestPlugin(t)
	request := sendToBob(t, p, "Sign the PO")
	_, _, err := p.acceptRequest(bob, request.ID, "")
	require.NoError(t, err)
	require.NoError(t, p.DeleteItem(bob, defaultPersonalListID(bob), request.ID))
	assert.Empty(t, sent(t, p, alice))
	assert.Contains(t, api.dms[alice][len(api.dms[alice])-1], "@bob declined a Todo you sent")

	items, err := p.getItems(defaultPersonalListID(bob))
	require.NoError(t, err)
	assert.Empty(t, items)
	data, _ := api.KVGet(issueKey(request.ID))
	assert.Nil(t, data, "bob's copy is gone too")
}

func TestTheSenderCancelsOrEditsAnAcceptedTodo(t *testing.T) {
	p, _ := newTestPlugin(t)
	request := sendToBob(t, p, "Sign the PO")
	_, _, err := p.acceptRequest(bob, request.ID, "")
	require.NoError(t, err)
	out := sent(t, p, alice)
	require.Len(t, out, 1)

	_, _, _, err = p.listManager.EditIssue(alice, out[0].ID, "Sign the PO by Friday", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"Sign the PO by Friday"}, messages(t, p, defaultPersonalListID(bob)), "edited in bob's list")

	_, _, _, _, err = p.listManager.RemoveIssue(alice, out[0].ID)
	require.NoError(t, err)
	assert.Empty(t, messages(t, p, defaultPersonalListID(bob)), "cancelled: it leaves bob's list")
}

func TestTheSenderReassignsAnAcceptedTodo(t *testing.T) {
	p, _ := newTestPlugin(t)
	request := sendToBob(t, p, "Sign the PO")
	_, _, err := p.acceptRequest(bob, request.ID, "")
	require.NoError(t, err)
	out := sent(t, p, alice)
	require.Len(t, out, 1)

	_, oldOwner, err := p.listManager.ChangeAssignment(out[0].ID, alice, carol)
	require.NoError(t, err)
	assert.Equal(t, bob, oldOwner)
	assert.Empty(t, messages(t, p, defaultPersonalListID(bob)), "it leaves bob's list")
	received, err := p.listManager.GetIssueList(carol, InListKey)
	require.NoError(t, err)
	assert.Len(t, received, 1, "for carol's requests")
}

func TestATodoReceivedIsNotSentOn(t *testing.T) {
	p, _ := newTestPlugin(t)
	request := sendToBob(t, p, "Sign the PO")
	_, _, err := p.acceptRequest(bob, request.ID, "")
	require.NoError(t, err)
	_, err = p.SendItem(bob, defaultPersonalListID(bob), request.ID, "carol")
	assert.Equal(t, http.StatusBadRequest, status(err))
}

// Regression: reminding the receiver of a todo they had accepted moved another of their requests
// to the top (the last one looked at).
func TestBumpingAnAcceptedTodoMovesNothing(t *testing.T) {
	p, _ := newTestPlugin(t)
	first := sendToBob(t, p, "First")
	sendToBob(t, p, "Second")
	_, _, err := p.acceptRequest(bob, first.ID, "")
	require.NoError(t, err)
	sendToBob(t, p, "Third")

	out := sent(t, p, alice)
	require.Len(t, out, 3)
	var firstSent string
	for _, issue := range out {
		if issue.Message == "First" {
			firstSent = issue.ID
		}
	}
	_, _, _, err = p.listManager.BumpIssue(alice, firstSent)
	assert.Error(t, err)
	received, err := p.listManager.GetIssueList(bob, InListKey)
	require.NoError(t, err)
	require.Len(t, received, 2)
	assert.Equal(t, []string{"Second", "Third"}, []string{received[0].Message, received[1].Message}, "unchanged")
}

func TestHTTPAcceptIntoAList(t *testing.T) {
	p, _ := newTestPlugin(t)
	request := sendToBob(t, p, "Sign the PO")
	w := call(t, p, carol, http.MethodPost, "/accept", map[string]string{"id": request.ID})
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = call(t, p, bob, http.MethodPost, "/accept", map[string]string{"id": request.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"Sign the PO"}, messages(t, p, defaultPersonalListID(bob)))
}
