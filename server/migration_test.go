// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The KV store of the versions before lists (0.8.x): alice's own todos (one she accepted from bob),
// a request from carol waiting for her answer, reader's todos in the oldest format (IDs, one of
// them dangling), the shared list of town (a todo assigned to bob), the lists alice follows, and a
// channel's list that can't be read.
func oldLayout(t *testing.T, api *fakeAPI) {
	t.Helper()
	issue := func(id, message string) {
		api.setJSON(t, "item_"+id, Issue{ID: id, Message: message, CreateAt: 100, PostPermalink: "", PostID: ""})
	}
	issue("i1", "Water the plants")
	issue("i2", "Review bob's deck")
	issue("bobout1", "Review bob's deck")
	issue("i3", "Sign carol's PO")
	issue("carolout1", "Sign carol's PO")
	issue("i4", "Call the bank")
	api.setJSON(t, "order_"+alice, []IssueRef{{IssueID: "i1"}, {IssueID: "i2", ForeignUserID: bob, ForeignIssueID: "bobout1"}})
	api.setJSON(t, "order_"+bob+"_out", []IssueRef{{IssueID: "bobout1", ForeignUserID: alice, ForeignIssueID: "i2"}})
	api.setJSON(t, "order_"+alice+"_in", []IssueRef{{IssueID: "i3", ForeignUserID: carol, ForeignIssueID: "carolout1"}})
	api.setJSON(t, "order_"+carol+"_out", []IssueRef{{IssueID: "carolout1", ForeignUserID: alice, ForeignIssueID: "i3"}})
	api.setJSON(t, "order_"+reader, []string{"i4", "i5"})

	api.setJSON(t, "chlist_"+town, []legacyChannelTodo{
		{ID: "t1", ChannelID: town, Message: "Order the bulbs", CreatorID: alice, CreateAt: 1, UpdateAt: 1, AssigneeID: bob},
		{ID: "t2", ChannelID: town, Message: "Book the room", CreatorID: bob, CreateAt: 2, UpdateAt: 3, DoneBy: carol, DoneAt: 3},
		{ID: "t3", ChannelID: town, Message: "Fix the OTA bug", CreatorID: carol, CreateAt: 4, UpdateAt: 4, PostID: townPost, PostPermalink: "http://chat.example/lumen/pl/" + townPost},
	})
	api.setJSON(t, "chassigned_"+bob, []assignedRef{{ChannelID: town, TodoID: "t1"}})
	api.setJSON(t, "chfollowing_"+alice, []string{town, news})
	require.Nil(t, api.KVSet("chlist_"+attic, []byte("not json")))
}

func itemIDs(t *testing.T, p *Plugin, listID string) []string {
	t.Helper()
	items, err := p.getItems(listID)
	require.NoError(t, err)
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestMigrationMakesListsOfTheOldTodos(t *testing.T) {
	p, api := newTestPlugin(t)
	oldLayout(t, api)
	require.NoError(t, p.MigrateAll())

	// alice's own todos are her default list, in their order, with their IDs
	lists, err := p.PersonalLists(alice)
	require.NoError(t, err)
	require.Len(t, lists, 1)
	assert.Equal(t, DefaultPersonalListName, lists[0].Name)
	assert.Equal(t, 2, lists[0].OpenCount)
	assert.Equal(t, []string{"i1", "i2"}, itemIDs(t, p, lists[0].ID))
	assert.Equal(t, []string{"Water the plants", "Review bob's deck"}, messages(t, p, lists[0].ID))
	items, err := p.getItems(lists[0].ID)
	require.NoError(t, err)
	assert.Equal(t, bob, items[1].FromUserID, "the todo accepted from bob stays linked to his copy")
	assert.Equal(t, "bobout1", items[1].FromIssueID)
	assert.Empty(t, items[0].FromUserID)

	// reader's todos, in the oldest format: the dangling one is skipped, and kept in the backup
	assert.Equal(t, []string{"i4"}, itemIDs(t, p, defaultPersonalListID(reader)))
	backups := api.keys(keyBackupPrefix + "order_" + reader + "_")
	require.Len(t, backups, 1)
	assert.JSONEq(t, `["i4","i5"]`, string(api.kv[backups[0]]))

	// town's todos are its first list, done ones under the open ones, still assigned
	channelLists, err := p.ChannelLists(bob, town)
	require.NoError(t, err)
	require.Len(t, channelLists, 1)
	assert.Equal(t, "Town Square todos", channelLists[0].Name)
	assert.Equal(t, defaultChannelListID(town), channelLists[0].ID)
	assert.Empty(t, channelLists[0].OwnerID, "nobody made it")
	assert.Equal(t, []string{"t1", "t3", "t2"}, itemIDs(t, p, channelLists[0].ID))
	detail, err := p.GetList(bob, channelLists[0].ID)
	require.NoError(t, err)
	assert.Equal(t, bob, detail.Items[0].AssigneeID)
	assert.Equal(t, carol, detail.Items[2].DoneBy)
	assert.Equal(t, townPost, detail.Items[1].PostID)
	assigned, err := p.AssignedTodos(bob)
	require.NoError(t, err)
	require.Len(t, assigned, 1, "bob's assignment of before lists still points to it")
	assert.Equal(t, "Town Square todos", assigned[0].ListName)

	// the channels alice followed are favorites: town's list (news had no list)
	favorites, err := p.FavoriteLists(alice)
	require.NoError(t, err)
	require.Len(t, favorites, 1)
	assert.Equal(t, defaultChannelListID(town), favorites[0].ID)

	// sent and received todos don't change
	waiting, err := p.listManager.GetIssueList(alice, InListKey)
	require.NoError(t, err)
	require.Len(t, waiting, 1)
	assert.Equal(t, "Sign carol's PO", waiting[0].Message)
	assert.Len(t, sent(t, p, bob), 1)

	// the old keys are gone, each backed up as it was; what couldn't be read is left as it is
	for _, key := range []string{"order_" + alice, "order_" + reader, "chlist_" + town, "chfollowing_" + alice} {
		assert.NotContains(t, api.kv, key)
		assert.Len(t, api.keys(keyBackupPrefix+key+"_"), 1, key)
	}
	assert.Equal(t, []byte("not json"), api.kv["chlist_"+attic])
	for _, key := range []string{"order_" + alice + "_in", "order_" + bob + "_out", "order_" + carol + "_out", "item_i1", "item_i3"} {
		assert.Contains(t, api.kv, key)
	}

	var record migrationRecord
	require.NoError(t, json.Unmarshal(api.kv[keyMigration], &record))
	assert.Equal(t, ListsDataVersion, record.Version)
	assert.NotContains(t, api.kv, keyMigrationLock)
}

func TestMigratedLinksToSendersStillWork(t *testing.T) {
	p, api := newTestPlugin(t)
	oldLayout(t, api)
	require.NoError(t, p.MigrateAll())

	_, err := p.UpdateItem(alice, defaultPersonalListID(alice), "i2", itemRequest{Done: yes()})
	require.NoError(t, err)
	assert.Empty(t, sent(t, p, bob), "done: it leaves bob's Sent by you")
	require.NotEmpty(t, api.dms[bob])
	assert.Contains(t, api.dms[bob][len(api.dms[bob])-1], "@alice completed a Todo you sent: Review bob's deck")

	// carol's request, accepted now, lands in alice's list too
	_, _, err = p.acceptRequest(alice, "i3", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"i1", "i3", "i2"}, itemIDs(t, p, defaultPersonalListID(alice)))
}

func TestMigrationNeverLosesATodo(t *testing.T) {
	p, api := newTestPlugin(t)
	oldLayout(t, api)
	before := map[string]bool{"i1": true, "i2": true, "i4": true, "t1": true, "t2": true, "t3": true}
	require.NoError(t, p.MigrateAll())

	after := map[string]bool{}
	for _, key := range api.keys(keyItemsPrefix) {
		var items []*Item
		require.NoError(t, json.Unmarshal(api.kv[key], &items))
		for _, item := range items {
			assert.False(t, after[item.ID], "%s once", item.ID)
			after[item.ID] = true
		}
	}
	assert.Equal(t, before, after)
}

func TestMigrationRunsOnceAndAgainSafely(t *testing.T) {
	p, api := newTestPlugin(t)
	oldLayout(t, api)
	require.NoError(t, p.MigrateAll())
	snapshot := map[string]string{}
	for key, value := range api.kv {
		snapshot[key] = string(value)
	}
	require.NoError(t, p.MigrateAll())
	for key, value := range api.kv {
		assert.Equal(t, snapshot[key], string(value), "%s unchanged by a second run", key)
	}

	// An older version, still running on another server, writes the old list again: migrated
	// again, without twice the same todo.
	api.setJSON(t, "order_"+alice, []IssueRef{{IssueID: "i1"}, {IssueID: "i2", ForeignUserID: bob, ForeignIssueID: "bobout1"}})
	_, err := p.PersonalLists(alice)
	require.NoError(t, err)
	assert.Equal(t, []string{"i1", "i2"}, itemIDs(t, p, defaultPersonalListID(alice)))
	assert.NotContains(t, api.kv, "order_"+alice)
}

func TestMigrationIsLazyToo(t *testing.T) {
	p, api := newTestPlugin(t)
	oldLayout(t, api)

	assigned, err := p.AssignedTodos(bob)
	require.NoError(t, err)
	require.Len(t, assigned, 1, "the channel's todos are migrated before the assignment is read")
	lists, err := p.PersonalLists(alice)
	require.NoError(t, err)
	assert.Equal(t, 2, lists[0].OpenCount)
	favorites, err := p.FavoriteLists(alice)
	require.NoError(t, err)
	assert.Len(t, favorites, 1)
	targets, err := p.Targets(reader, news)
	require.NoError(t, err)
	assert.Equal(t, defaultPersonalListID(reader), targets.DefaultID)
	assert.Equal(t, []string{"i4"}, itemIDs(t, p, defaultPersonalListID(reader)))
	assert.NotContains(t, api.kv, keyMigration, "the sweep hasn't run")
}

func TestMigrationMergesWhatChangedMeanwhile(t *testing.T) {
	p, api := newTestPlugin(t)
	oldLayout(t, api)
	api.setJSON(t, "item_i6", Issue{ID: "i6", Message: "Added meanwhile", CreateAt: 200})
	changed := false
	api.beforeCompareAndDelete = func(key string) {
		if key == "order_"+alice && !changed {
			changed = true
			api.setJSON(t, key, []IssueRef{{IssueID: "i1"}, {IssueID: "i2", ForeignUserID: bob, ForeignIssueID: "bobout1"}, {IssueID: "i6"}})
		}
	}
	require.NoError(t, p.migratePersonal(alice))
	assert.True(t, changed)
	assert.Equal(t, []string{"i1", "i2", "i6"}, itemIDs(t, p, defaultPersonalListID(alice)))
	assert.NotContains(t, api.kv, "order_"+alice)
	backups := api.keys(keyBackupPrefix + "order_" + alice + "_")
	require.Len(t, backups, 2, "both versions are backed up")
	withI6 := 0
	for _, key := range backups {
		if strings.Contains(string(api.kv[key]), `"i6"`) {
			withI6++
		}
	}
	assert.Equal(t, 1, withI6)
}

func TestMigrationWaitsForAnotherServer(t *testing.T) {
	p, api := newTestPlugin(t)
	oldLayout(t, api)
	require.Nil(t, api.KVSet(keyMigrationLock, []byte("1")))
	require.NoError(t, p.MigrateAll())
	assert.Contains(t, api.kv, "chlist_"+town, "another server is migrating")
	assert.NotContains(t, api.kv, keyMigration)
}

func TestDerivedListIDs(t *testing.T) {
	id := defaultPersonalListID(alice)
	assert.Len(t, id, 26)
	assert.Regexp(t, "^[a-z0-9]{26}$", id, "like the routes' IDs")
	assert.Equal(t, id, defaultPersonalListID(alice), "always the same")
	assert.NotEqual(t, id, defaultPersonalListID(bob))
	assert.NotEqual(t, defaultChannelListID(town), defaultPersonalListID(town), "a channel's isn't a person's")
}
