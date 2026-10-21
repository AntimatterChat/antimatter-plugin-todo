// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEveryoneHasADefaultPersonalList(t *testing.T) {
	p, _ := newTestPlugin(t)
	lists, err := p.PersonalLists(alice)
	require.NoError(t, err)
	require.Len(t, lists, 1)
	mine := lists[0]
	assert.Equal(t, DefaultPersonalListName, mine.Name)
	assert.Equal(t, ScopePersonal, mine.Scope)
	assert.True(t, mine.Default)
	assert.True(t, mine.CanEdit)
	assert.False(t, mine.CanDelete, "the default list stays")
	assert.Equal(t, defaultPersonalListID(alice), mine.ID)

	again, err := p.PersonalLists(alice)
	require.NoError(t, err)
	assert.Len(t, again, 1, "made once")

	_, err = p.UpdateList(alice, mine.ID, listRequest{Archived: yes()})
	assert.Equal(t, http.StatusBadRequest, status(err), "nor archived")
	assert.Equal(t, http.StatusBadRequest, status(p.DeleteList(alice, mine.ID)), "nor deleted")
	renamed, err := p.UpdateList(alice, mine.ID, listRequest{Name: str("  Mes   tâches ")})
	require.NoError(t, err)
	assert.Equal(t, "Mes tâches", renamed.Name, "but renamed")
}

func TestPersonalListsAreTheirOwnersAlone(t *testing.T) {
	p, _ := newTestPlugin(t)
	groceries := newList(t, p, alice, "", "Groceries")
	addItem(t, p, alice, groceries.ID, "Oat milk")

	for _, other := range []string{bob, carol, guest, outsider} {
		_, err := p.GetList(other, groceries.ID)
		assert.Equal(t, http.StatusNotFound, status(err), "%s doesn't see it, nor learns it exists", other)
		_, err = p.AddItem(other, groceries.ID, itemRequest{Message: str("x")})
		assert.Equal(t, http.StatusNotFound, status(err))
		_, err = p.UpdateList(other, groceries.ID, listRequest{Name: str("Mine now")})
		assert.Equal(t, http.StatusNotFound, status(err))
		assert.Equal(t, http.StatusNotFound, status(p.DeleteList(other, groceries.ID)))
		assert.Equal(t, http.StatusNotFound, status(p.SetFavorite(other, groceries.ID, true)))
		lists, err := p.PersonalLists(other)
		require.NoError(t, err)
		for _, list := range lists {
			assert.NotEqual(t, groceries.ID, list.ID)
		}
	}
	items, err := p.getItems(groceries.ID)
	require.NoError(t, err)
	_, err = p.UpdateItem(bob, groceries.ID, items[0].ID, itemRequest{Done: yes()})
	assert.Equal(t, http.StatusNotFound, status(err))
	assert.Equal(t, http.StatusNotFound, status(p.DeleteItem(bob, groceries.ID, items[0].ID)))
	assert.Equal(t, []string{"Oat milk"}, messages(t, p, groceries.ID))
}

// The rights on a channel's lists: a member, a non-member, a guest in the channel and one in no
// channel, someone who only reads the channel, and an archived channel.
func TestChannelListRights(t *testing.T) {
	p, api := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	headlines := newList(t, p, alice, news, "Headlines")
	addItem(t, p, alice, launch.ID, "Book the room")
	addItem(t, p, alice, headlines.ID, "Proofread")
	// The archived channel's list, from before it was archived
	api.channels[attic].DeleteAt = 0
	boxes := newList(t, p, alice, attic, "Boxes")
	addItem(t, p, alice, boxes.ID, "Label them")
	api.channels[attic].DeleteAt = 1

	cases := []struct {
		name      string
		userID    string
		listID    string
		channelID string
		read      bool
		edit      bool
	}{
		{"a member", bob, launch.ID, town, true, true},
		{"a guest in the channel", guest, launch.ID, town, true, true},
		{"a non-member", outsider, launch.ID, town, false, false},
		{"a guest in no channel", visitor, launch.ID, town, false, false},
		{"a member of another channel", reader, launch.ID, town, false, false},
		{"a member who only reads", reader, headlines.ID, news, true, false},
		{"a member of an archived channel", bob, boxes.ID, attic, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lists, err := p.ChannelLists(c.userID, c.channelID)
			list, getErr := p.GetList(c.userID, c.listID)
			if !c.read {
				assert.Equal(t, http.StatusForbidden, status(err), "the channel's lists are hidden")
				assert.Equal(t, http.StatusNotFound, status(getErr), "and so is the list")
				_, err = p.CreateList(c.userID, listRequest{Name: str("x"), ChannelID: c.channelID})
				assert.Equal(t, http.StatusForbidden, status(err))
				_, err = p.AddItem(c.userID, c.listID, itemRequest{Message: str("x")})
				assert.Equal(t, http.StatusNotFound, status(err))
				assert.Equal(t, http.StatusNotFound, status(p.SetFavorite(c.userID, c.listID, true)))
				return
			}
			require.NoError(t, err)
			require.NoError(t, getErr)
			assert.NotEmpty(t, lists)
			assert.NotEmpty(t, list.Items)
			assert.Equal(t, c.edit, list.CanEdit)
			assert.Equal(t, c.edit, list.CanRename)

			_, err = p.AddItem(c.userID, c.listID, itemRequest{Message: str("Another")})
			_, createErr := p.CreateList(c.userID, listRequest{Name: str("Another"), ChannelID: c.channelID})
			_, renameErr := p.UpdateList(c.userID, c.listID, listRequest{Name: str("Renamed")})
			doneErr := func() error {
				_, updateErr := p.UpdateItem(c.userID, c.listID, list.Items[0].ID, itemRequest{Done: yes()})
				return updateErr
			}()
			if c.edit {
				assert.NoError(t, err)
				assert.NoError(t, createErr)
				assert.NoError(t, renameErr)
				assert.NoError(t, doneErr)
			} else {
				assert.Equal(t, http.StatusForbidden, status(err), "reads, doesn't add")
				assert.Equal(t, http.StatusForbidden, status(createErr), "nor makes lists")
				assert.Equal(t, http.StatusForbidden, status(renameErr), "nor renames them")
				assert.Equal(t, http.StatusForbidden, status(doneErr), "nor checks todos off")
				assert.Equal(t, http.StatusForbidden, status(p.DeleteList(c.userID, c.listID)))
			}
		})
	}
}

func TestLeavingAChannelHidesItsLists(t *testing.T) {
	p, api := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	require.NoError(t, p.SetFavorite(bob, launch.ID, true))
	favorites, err := p.FavoriteLists(bob)
	require.NoError(t, err)
	require.Len(t, favorites, 1)

	api.leave(town, bob)
	_, err = p.GetList(bob, launch.ID)
	assert.Equal(t, http.StatusNotFound, status(err))
	favorites, err = p.FavoriteLists(bob)
	require.NoError(t, err)
	assert.Empty(t, favorites, "a favorite one can't read anymore is hidden")
}

func TestListLifecycle(t *testing.T) {
	p, _ := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	assert.Equal(t, ScopeChannel, launch.Scope)
	assert.Equal(t, "Town Square", launch.ChannelName)
	assert.Equal(t, "lumen", launch.TeamName)
	retro := newList(t, p, bob, town, "Retro")

	lists, err := p.ChannelLists(carol, town)
	require.NoError(t, err)
	require.Len(t, lists, 2)
	assert.Equal(t, []string{"Launch", "Retro"}, []string{lists[0].Name, lists[1].Name}, "in the order they were made")

	_, err = p.UpdateList(bob, retro.ID, listRequest{Position: at(0)})
	require.NoError(t, err)
	lists, err = p.ChannelLists(carol, town)
	require.NoError(t, err)
	assert.Equal(t, "Retro", lists[0].Name, "moved first")

	updated, err := p.UpdateList(bob, launch.ID, listRequest{Name: str("Launch day"), Icon: str("🚀")})
	require.NoError(t, err, "any member renames a channel's list")
	assert.Equal(t, "Launch day", updated.Name)
	assert.Equal(t, "🚀", updated.Icon)
	for _, icon := range []string{"abc", "1", " x ", "🚀🚀🚀🚀🚀🚀🚀🚀🚀"} {
		_, err = p.UpdateList(bob, launch.ID, listRequest{Icon: str(icon)})
		assert.Equal(t, http.StatusBadRequest, status(err), "%q isn't an emoji", icon)
	}
	_, err = p.UpdateList(bob, launch.ID, listRequest{Name: str("   ")})
	assert.Equal(t, http.StatusBadRequest, status(err))
	_, err = p.UpdateList(bob, launch.ID, listRequest{Name: str(strings.Repeat("x", maxListName+1))})
	assert.Equal(t, http.StatusBadRequest, status(err))

	item := addItem(t, p, alice, launch.ID, "Book the room")
	archived, err := p.UpdateList(alice, launch.ID, listRequest{Archived: yes()})
	require.NoError(t, err)
	assert.NotZero(t, archived.ArchivedAt)
	assert.False(t, archived.CanEdit, "an archived list is read-only")
	assert.True(t, archived.CanRename, "but can be unarchived")
	_, err = p.AddItem(alice, launch.ID, itemRequest{Message: str("x")})
	assert.Equal(t, http.StatusForbidden, status(err))
	_, err = p.UpdateItem(alice, launch.ID, item.ID, itemRequest{Done: yes()})
	assert.Equal(t, http.StatusForbidden, status(err))
	lists, err = p.ChannelLists(carol, town)
	require.NoError(t, err)
	assert.Len(t, lists, 2, "still listed, with its archive date")

	unarchived, err := p.UpdateList(alice, launch.ID, listRequest{Archived: no()})
	require.NoError(t, err)
	assert.Zero(t, unarchived.ArchivedAt)
	assert.True(t, unarchived.CanEdit)

	assert.Equal(t, http.StatusForbidden, status(p.DeleteList(bob, launch.ID)), "only its creator or a channel admin deletes it")
	require.NoError(t, p.DeleteList(alice, launch.ID))
	require.NoError(t, p.DeleteList(carol, retro.ID), "a channel admin deletes anyone's")
	lists, err = p.ChannelLists(carol, town)
	require.NoError(t, err)
	assert.Empty(t, lists)
	_, err = p.GetList(alice, launch.ID)
	assert.Equal(t, http.StatusNotFound, status(err))
	data, _ := p.API.KVGet(itemsKey(launch.ID))
	assert.Nil(t, data, "its todos are gone")
}

// Who deletes a channel's lists and todos: their creators and the channel's admins (channel,
// community and system admins), not every member who may rename the channel, which on a new server
// is every member. The other members still add, change and check off todos.
func TestOnlyCreatorsAndAdminsDelete(t *testing.T) {
	p, _ := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	room := addItem(t, p, alice, launch.ID, "Book the room")

	view, err := p.GetList(bob, launch.ID)
	require.NoError(t, err)
	assert.False(t, view.CanDelete, "a member who didn't make the list")
	assert.False(t, view.CanManage)
	assert.True(t, view.CanEdit)
	assert.Equal(t, http.StatusForbidden, status(p.DeleteList(bob, launch.ID)), "a member doesn't delete someone else's list")
	assert.Equal(t, http.StatusForbidden, status(p.DeleteItem(bob, launch.ID, room.ID)), "nor someone else's todo")

	_, err = p.UpdateItem(bob, launch.ID, room.ID, itemRequest{Message: str("Book the big room"), Done: yes()})
	require.NoError(t, err, "but changes and checks off todos")
	cake := addItem(t, p, bob, launch.ID, "Order the cake")
	require.NoError(t, p.DeleteItem(bob, launch.ID, cake.ID), "and deletes their own")

	view, err = p.GetList(alice, launch.ID)
	require.NoError(t, err)
	assert.True(t, view.CanDelete, "the list's creator")
	assert.False(t, view.CanManage)

	for _, admin := range []struct{ name, userID string }{{"a channel admin", carol}, {"a community admin", dana}, {"a system admin", sam}} {
		t.Run(admin.name, func(t *testing.T) {
			list := newList(t, p, bob, town, "Retro")
			item := addItem(t, p, bob, list.ID, "Collect feedback")
			view, err := p.GetList(admin.userID, list.ID)
			require.NoError(t, err)
			assert.True(t, view.CanDelete)
			assert.True(t, view.CanManage)
			require.NoError(t, p.DeleteItem(admin.userID, list.ID, item.ID), "deletes someone else's todo")
			require.NoError(t, p.DeleteList(admin.userID, list.ID), "and list")
		})
	}

	require.NoError(t, p.DeleteList(alice, launch.ID), "the creator deletes their list")
}

func TestConversationLists(t *testing.T) {
	p, _ := newTestPlugin(t)
	plans := newList(t, p, alice, dmAB, "Plans")
	assert.Equal(t, ScopeConversation, plans.Scope)
	assert.Empty(t, plans.ChannelName, "a conversation's name is its people's, not shown")
	assert.Equal(t, http.StatusForbidden, status(p.DeleteList(bob, plans.ID)), "nobody manages a conversation: its creator deletes it")
	_, err := p.GetList(carol, plans.ID)
	assert.Equal(t, http.StatusNotFound, status(err))
	require.NoError(t, p.DeleteList(alice, plans.ID))
}

func TestListsPerScopeAreCapped(t *testing.T) {
	p, _ := newTestPlugin(t)
	for i := range maxListsPerScope - 1 {
		newList(t, p, alice, "", "List "+string(rune('A'+i%26)))
	}
	_, err := p.CreateList(alice, listRequest{Name: str("One too many")})
	assert.Equal(t, http.StatusBadRequest, status(err), "the default list counts")
	assert.Len(t, p.API.(*fakeAPI).keys(keyListPrefix), maxListsPerScope, "and nothing is left behind")
}

func TestFavorites(t *testing.T) {
	p, _ := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	groceries := newList(t, p, alice, "", "Groceries")
	require.NoError(t, p.SetFavorite(alice, launch.ID, true))
	require.NoError(t, p.SetFavorite(alice, groceries.ID, true))
	require.NoError(t, p.SetFavorite(alice, groceries.ID, true))

	favorites, err := p.FavoriteLists(alice)
	require.NoError(t, err)
	require.Len(t, favorites, 2, "starring twice stars once")
	assert.Equal(t, "Groceries", favorites[0].Name, "the latest first")
	assert.True(t, favorites[0].Favorite)
	assert.Equal(t, ScopePersonal, favorites[0].Scope)
	assert.Equal(t, ScopeChannel, favorites[1].Scope)

	detail, err := p.GetList(bob, launch.ID)
	require.NoError(t, err)
	assert.False(t, detail.Favorite, "favorites are each one's own")

	require.NoError(t, p.SetFavorite(alice, launch.ID, false))
	favorites, err = p.FavoriteLists(alice)
	require.NoError(t, err)
	assert.Len(t, favorites, 1)

	require.NoError(t, p.DeleteList(alice, groceries.ID))
	favorites, err = p.FavoriteLists(alice)
	require.NoError(t, err)
	assert.Empty(t, favorites, "a deleted list leaves the favorites")
}

func TestOpenCountAndEarliestDue(t *testing.T) {
	p, _ := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	first := addItem(t, p, alice, launch.ID, "Book the room")
	_, err := p.AddItem(alice, launch.ID, itemRequest{Message: str("Print the badges"), Due: str("2026-11-02")})
	require.NoError(t, err)
	_, err = p.AddItem(alice, launch.ID, itemRequest{Message: str("Order the cake"), Due: str("2026-10-20")})
	require.NoError(t, err)
	_, err = p.UpdateItem(alice, launch.ID, first.ID, itemRequest{Due: str("2026-10-01"), Done: yes()})
	require.NoError(t, err)

	lists, err := p.ChannelLists(bob, town)
	require.NoError(t, err)
	require.Len(t, lists, 1)
	assert.Equal(t, 2, lists[0].OpenCount)
	assert.Equal(t, "2026-10-20", lists[0].EarliestDue, "of the open todos")

	for _, due := range []string{"tomorrow", "2026-13-01", "2026-02-30", "26-10-20"} {
		_, err = p.AddItem(alice, launch.ID, itemRequest{Message: str("x"), Due: str(due)})
		assert.Equal(t, http.StatusBadRequest, status(err), due)
	}
}

func TestTargetsOfferTheLastListUsed(t *testing.T) {
	p, _ := newTestPlugin(t)
	targets, err := p.Targets(alice, town)
	require.NoError(t, err)
	assert.Equal(t, defaultPersonalListID(alice), targets.DefaultID, "no list in the channel: my todos")

	launch := newList(t, p, alice, town, "Launch")
	retro := newList(t, p, bob, town, "Retro")
	newList(t, p, alice, news, "Headlines")
	targets, err = p.Targets(alice, town)
	require.NoError(t, err)
	assert.Equal(t, launch.ID, targets.DefaultID, "then the channel's first list")
	names := []string{}
	for _, list := range targets.Lists {
		names = append(names, list.Name)
	}
	assert.Equal(t, []string{"Launch", "Retro", DefaultPersonalListName}, names, "the channel's lists and mine, not another channel's")

	_, err = p.AddItem(alice, retro.ID, itemRequest{Message: str("Bring snacks"), ChannelID: town})
	require.NoError(t, err)
	targets, err = p.Targets(alice, town)
	require.NoError(t, err)
	assert.Equal(t, retro.ID, targets.DefaultID, "then the one I last added to from here")
	targets, err = p.Targets(bob, town)
	require.NoError(t, err)
	assert.Equal(t, launch.ID, targets.DefaultID, "mine, not bob's")

	_, err = p.UpdateList(alice, retro.ID, listRequest{Archived: yes()})
	require.NoError(t, err)
	targets, err = p.Targets(alice, town)
	require.NoError(t, err)
	assert.Equal(t, launch.ID, targets.DefaultID, "not an archived list")

	targets, err = p.Targets(reader, news)
	require.NoError(t, err)
	assert.Equal(t, defaultPersonalListID(reader), targets.DefaultID, "not a list one can't add to")
	require.Len(t, targets.Lists, 1)

	// From a channel, a list of another channel added to from there (a favorite) comes first
	headlines, err := p.ChannelLists(alice, news)
	require.NoError(t, err)
	_, err = p.AddItem(alice, headlines[0].ID, itemRequest{Message: str("Proofread"), ChannelID: town})
	require.NoError(t, err)
	targets, err = p.Targets(alice, town)
	require.NoError(t, err)
	assert.Equal(t, headlines[0].ID, targets.DefaultID)
}

func TestHTTPListsAPI(t *testing.T) {
	p, _ := newTestPlugin(t)
	w := call(t, p, "", http.MethodGet, "/api/v1/lists", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "nobody")

	w = call(t, p, alice, http.MethodPost, "/api/v1/lists", map[string]string{"name": "Launch", "channel_id": town})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	launch := decodeID(t, w.Body.Bytes())

	w = call(t, p, outsider, http.MethodGet, "/api/v1/lists?scope=channel&channel_id="+town, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = call(t, p, outsider, http.MethodGet, "/api/v1/lists/"+launch, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = call(t, p, outsider, http.MethodPost, "/api/v1/lists/"+launch+"/items", map[string]string{"message": "x"})
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = call(t, p, bob, http.MethodPost, "/api/v1/lists/"+launch+"/items", map[string]any{"message": "Book the room", "due": "2026-10-20"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	w = call(t, p, bob, http.MethodGet, "/api/v1/lists?scope=channel&channel_id="+town, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"open_count":1`)
	w = call(t, p, bob, http.MethodPut, "/api/v1/lists/"+launch+"/favorite", nil)
	require.Equal(t, http.StatusOK, w.Code)
	w = call(t, p, bob, http.MethodGet, "/api/v1/lists?scope=favorites", nil)
	assert.Contains(t, w.Body.String(), launch)
	w = call(t, p, bob, http.MethodGet, "/api/v1/targets?channel_id="+town, nil)
	assert.Contains(t, w.Body.String(), `"default_id":"`+launch+`"`)
	w = call(t, p, bob, http.MethodDelete, "/api/v1/lists/"+launch, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = call(t, p, alice, http.MethodDelete, "/api/v1/lists/"+launch, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	w = call(t, p, alice, http.MethodGet, "/api/v1/lists?scope=nope", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func decodeID(t *testing.T, body []byte) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &v))
	require.NotEmpty(t, v.ID)
	return v.ID
}
