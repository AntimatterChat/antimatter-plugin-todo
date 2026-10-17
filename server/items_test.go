// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"net/http"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	townPost   = "p0000000000000000000000000"
	secretPost = "q0000000000000000000000000" // in a channel alice and bob aren't in
)

func TestItemsDoneMovedAndEdited(t *testing.T) {
	p, _ := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	first, err := p.AddItem(alice, launch.ID, itemRequest{Message: str("  a  ")})
	require.NoError(t, err)
	assert.Equal(t, "a", first.Message)
	addItem(t, p, bob, launch.ID, "b")
	c := addItem(t, p, alice, launch.ID, "c")
	assert.Equal(t, []string{"a", "b", "c"}, messages(t, p, launch.ID), "added at the end of the open todos")

	done, err := p.UpdateItem(bob, launch.ID, first.ID, itemRequest{Done: yes()})
	require.NoError(t, err)
	assert.Equal(t, bob, done.DoneBy)
	assert.Equal(t, []string{"b", "c", "a"}, messages(t, p, launch.ID), "done todos go under the open ones")
	addItem(t, p, alice, launch.ID, "d")
	assert.Equal(t, []string{"b", "c", "d", "a"}, messages(t, p, launch.ID), "and new ones over the done ones")

	_, err = p.UpdateItem(alice, launch.ID, c.ID, itemRequest{Position: at(0)})
	require.NoError(t, err)
	assert.Equal(t, []string{"c", "b", "d", "a"}, messages(t, p, launch.ID))

	reopened, err := p.UpdateItem(alice, launch.ID, first.ID, itemRequest{Done: no()})
	require.NoError(t, err)
	assert.Empty(t, reopened.DoneBy)
	assert.Equal(t, []string{"c", "b", "d", "a"}, messages(t, p, launch.ID), "reopened at the end of the open ones")

	edited, err := p.UpdateItem(bob, launch.ID, c.ID, itemRequest{Message: str("see"), Description: str("with honey"), Due: str("2026-10-20")})
	require.NoError(t, err)
	assert.Equal(t, "with honey", edited.Description)
	assert.Equal(t, "2026-10-20", edited.Due)
	cleared, err := p.UpdateItem(bob, launch.ID, c.ID, itemRequest{Due: str("")})
	require.NoError(t, err)
	assert.Empty(t, cleared.Due)

	_, err = p.UpdateItem(alice, launch.ID, c.ID, itemRequest{Message: str("  ")})
	assert.Equal(t, http.StatusBadRequest, status(err))
	_, err = p.UpdateItem(alice, launch.ID, "z0000000000000000000000000", itemRequest{Done: yes()})
	assert.Equal(t, http.StatusNotFound, status(err))
	_, err = p.AddItem(alice, launch.ID, itemRequest{Message: str("   ")})
	assert.Equal(t, http.StatusBadRequest, status(err))
}

func TestDoneTodosAreKeptUpToALimit(t *testing.T) {
	var items []*Item
	for i := range maxDoneItems + 5 {
		items = append(items, &Item{ID: model.NewId(), DoneAt: int64(i + 1)})
	}
	items = append(items, &Item{ID: "open"})
	sorted := sortItems(items)
	require.Len(t, sorted, maxDoneItems+1)
	assert.Equal(t, "open", sorted[0].ID)
	assert.EqualValues(t, maxDoneItems+5, sorted[1].DoneAt, "newest done first")
}

func TestDeletingTodos(t *testing.T) {
	p, _ := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	mine, err := p.AddItem(alice, launch.ID, itemRequest{Message: str("mine"), AssigneeID: str(bob)})
	require.NoError(t, err)
	other := addItem(t, p, alice, launch.ID, "other")

	assert.Equal(t, http.StatusForbidden, status(p.DeleteItem(bob, launch.ID, mine.ID)), "only its author or a channel admin")
	require.NoError(t, p.DeleteItem(alice, launch.ID, mine.ID))
	assigned, err := p.AssignedTodos(bob)
	require.NoError(t, err)
	assert.Empty(t, assigned)
	require.NoError(t, p.DeleteItem(carol, launch.ID, other.ID))
	assert.Empty(t, messages(t, p, launch.ID))

	groceries := newList(t, p, alice, "", "Groceries")
	milk := addItem(t, p, alice, groceries.ID, "Oat milk")
	require.NoError(t, p.DeleteItem(alice, groceries.ID, milk.ID), "one's own lists' todos")
}

func TestAssignments(t *testing.T) {
	p, api := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	todo, err := p.AddItem(alice, launch.ID, itemRequest{Message: str("Ship 2.4"), AssigneeID: str(bob), Due: str("2026-10-30")})
	require.NoError(t, err)
	assigned, err := p.AssignedTodos(bob)
	require.NoError(t, err)
	require.Len(t, assigned, 1)
	assert.Equal(t, "Launch", assigned[0].ListName)
	assert.Equal(t, "Town Square", assigned[0].ChannelName)
	assert.Equal(t, "lumen", assigned[0].TeamName)
	require.Len(t, api.dms[bob], 1)
	assert.Contains(t, api.dms[bob][0], "@alice assigned you a todo in Launch (~town-square)")
	assert.Contains(t, api.dms[bob][0], "Due 2026-10-30")

	for _, nobody := range []string{outsider, visitor, reader} {
		_, err = p.UpdateItem(alice, launch.ID, todo.ID, itemRequest{AssigneeID: str(nobody)})
		assert.Equal(t, http.StatusBadRequest, status(err), "only members of the channel can be assigned")
	}
	_, err = p.AddItem(alice, launch.ID, itemRequest{Message: str("x"), AssigneeID: str(outsider)})
	assert.Equal(t, http.StatusBadRequest, status(err))
	_, err = p.UpdateItem(alice, launch.ID, todo.ID, itemRequest{AssigneeID: str(guest)})
	require.NoError(t, err, "a guest in the channel can be")

	groceries := newList(t, p, alice, "", "Groceries")
	_, err = p.AddItem(alice, groceries.ID, itemRequest{Message: str("x"), AssigneeID: str(bob)})
	assert.Equal(t, http.StatusBadRequest, status(err), "a personal list's todos are sent, not assigned")

	_, err = p.UpdateItem(guest, launch.ID, todo.ID, itemRequest{AssigneeID: str(bob)})
	require.NoError(t, err)
	_, err = p.UpdateItem(bob, launch.ID, todo.ID, itemRequest{Done: yes()})
	require.NoError(t, err)
	assigned, err = p.AssignedTodos(bob)
	require.NoError(t, err)
	assert.Empty(t, assigned, "done todos leave the assignee's list")

	_, err = p.UpdateItem(bob, launch.ID, todo.ID, itemRequest{Done: no()})
	require.NoError(t, err)
	assigned, err = p.AssignedTodos(bob)
	require.NoError(t, err)
	assert.Len(t, assigned, 1, "and come back when reopened")
	assert.Len(t, api.dms[bob], 2, "without telling the assignee again")

	_, err = p.UpdateList(alice, launch.ID, listRequest{Archived: yes()})
	require.NoError(t, err)
	assigned, err = p.AssignedTodos(bob)
	require.NoError(t, err)
	assert.Empty(t, assigned, "hidden while the list is archived")
	_, err = p.UpdateList(alice, launch.ID, listRequest{Archived: no()})
	require.NoError(t, err)
	assigned, err = p.AssignedTodos(bob)
	require.NoError(t, err)
	assert.Len(t, assigned, 1, "and back once it's not")

	api.leave(town, bob)
	assigned, err = p.AssignedTodos(bob)
	require.NoError(t, err)
	assert.Empty(t, assigned, "not from a channel one left")
	assert.JSONEq(t, "[]", string(api.kv[assignedKey(bob)]), "which is forgotten")
}

func TestAssignmentsOfADeletedList(t *testing.T) {
	p, api := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	_, err := p.AddItem(alice, launch.ID, itemRequest{Message: str("Ship 2.4"), AssigneeID: str(bob)})
	require.NoError(t, err)
	require.NoError(t, p.DeleteList(alice, launch.ID))
	assigned, err := p.AssignedTodos(bob)
	require.NoError(t, err)
	assert.Empty(t, assigned)
	assert.JSONEq(t, "[]", string(api.kv[assignedKey(bob)]))
}

func TestTodosFromMessages(t *testing.T) {
	p, api := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	todo, err := p.AddItem(alice, launch.ID, itemRequest{Message: str("Order the bulbs"), PostID: townPost})
	require.NoError(t, err)
	assert.Equal(t, "http://chat.example/lumen/pl/"+townPost, todo.PostPermalink, "the server makes the link")
	require.Len(t, api.replies, 1)
	assert.Contains(t, api.replies[0].Message, "@alice added this to the todo list Launch")

	_, err = p.AddItem(alice, launch.ID, itemRequest{Message: str("x"), PostID: secretPost})
	assert.Equal(t, http.StatusBadRequest, status(err), "not a message one can't read")
	_, err = p.AddItem(alice, launch.ID, itemRequest{Message: str("x"), PostID: "n0000000000000000000000000"})
	assert.Equal(t, http.StatusBadRequest, status(err))

	headlines := newList(t, p, alice, news, "Headlines")
	_, err = p.AddItem(alice, headlines.ID, itemRequest{Message: str("Order the bulbs"), PostID: townPost})
	require.NoError(t, err, "a message of another channel can go to a list")
	assert.Len(t, api.replies, 1, "but its thread isn't told of a list its readers may not see")

	groceries := newList(t, p, alice, "", "Groceries")
	_, err = p.AddItem(alice, groceries.ID, itemRequest{Message: str("Order the bulbs"), PostID: townPost})
	require.NoError(t, err)
	require.Len(t, api.replies, 2)
	assert.Contains(t, api.replies[1].Message, "@alice attached a todo to this thread")
}

// Regression: the API of before lists took any message's ID, and the bot answered in its thread,
// even in a channel the person couldn't read.
func TestOldAddEndpointChecksTheMessage(t *testing.T) {
	p, api := newTestPlugin(t)
	w := call(t, p, alice, http.MethodPost, "/add", map[string]string{"message": "Leak", "post_id": secretPost, "postPermalink": "http://evil"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = call(t, p, alice, http.MethodPost, "/add", map[string]string{"message": "Leak", "post_id": secretPost, "send_to": "bob"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, api.replies)
	assert.Empty(t, api.dms[bob])

	w = call(t, p, alice, http.MethodPost, "/add", map[string]string{"message": "Order the bulbs", "post_id": townPost})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"Order the bulbs"}, messages(t, p, defaultPersonalListID(alice)), "into my todos")
	items, err := p.getItems(defaultPersonalListID(alice))
	require.NoError(t, err)
	assert.Equal(t, "http://chat.example/lumen/pl/"+townPost, items[0].PostPermalink)
}
