// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/pkg/errors"
)

// Before lists, someone's own todos were one list (order_<user>, with the todos in item_<id>), a
// channel had one shared list (chlist_<channel>), and people followed channels' lists
// (chfollowing_<user>). The migration (version 1) makes:
//
//   - of someone's own todos, their default personal list, "My todos" (the todos keep their IDs;
//     those they had accepted from others stay linked to their senders);
//   - of a channel's todos, its first list, "<channel> todos" (assignments keep pointing to them:
//     that list's ID is derived from the channel's);
//   - of the channels someone followed, favorites: those channels' lists.
//
// Each old key is copied under tlbackup_v1_<key>_<time>_<random> before it's deleted, and only deleted if
// nobody changed it meanwhile: no todo is lost, even if an older version of the plugin still runs
// on another server of a cluster during an upgrade. Todos sent and received (order_<user>_in and
// _out) don't change.
//
// The migration runs when someone first opens their lists or a channel's (lazily, so that it's
// done before anything is read), and for everyone in the background when the plugin starts; that
// sweep is recorded in tlmigration, with the version of the data.

const (
	// ListsDataVersion is the version of the data the migration brings the KV store to.
	ListsDataVersion = 1

	keyMigration     = "tlmigration"
	keyMigrationLock = "tlmigration_lock"
	keyBackupPrefix  = "tlbackup_v1_"

	oldChannelListPrefix = "chlist_"
	oldFollowingPrefix   = "chfollowing_"

	migrationLockSeconds = 15 * 60
	kvListPage           = 200
)

// The alphabet of model.NewId, so that derived IDs look like (and match the routes of) others.
var idEncoding = base32.NewEncoding("ybndrfg8ejkmcpqxot1uwisza345h769").WithPadding(base32.NoPadding)

// derivedID is a fixed list ID for a scope's default list: whoever makes it, at the same time or
// not, makes the same list.
func derivedID(kind, id string) string {
	sum := sha256.Sum256([]byte("todo-list:" + kind + ":" + id))
	return idEncoding.EncodeToString(sum[:16])
}

func defaultPersonalListID(userID string) string   { return derivedID(ScopePersonal, userID) }
func defaultChannelListID(channelID string) string { return derivedID(ScopeChannel, channelID) }

// legacyChannelTodo is a todo of a channel's list before lists.
type legacyChannelTodo struct {
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

type migrationRecord struct {
	Version int   `json:"version"`
	DoneAt  int64 `json:"done_at"`
}

// ensureList makes a list unless it exists, and puts it first in the index of its scope.
func (p *Plugin) ensureList(list *TodoList) (*TodoList, error) {
	stored, err := p.getList(list.ID)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		// Made here, or by whoever got there first
		if _, err = kvCreateJSON(p.API, todoListKey(list.ID), list); err != nil {
			return nil, err
		}
		if stored, err = p.getList(list.ID); err != nil {
			return nil, err
		}
		if stored == nil {
			return nil, errors.New("cannot create the list")
		}
	}
	if _, err = updateKV(p.API, scopeKey(stored), func(ids *[]string) error {
		if !slices.Contains(*ids, stored.ID) {
			*ids = append([]string{stored.ID}, *ids...)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return stored, nil
}

// ensurePersonal returns someone's default personal list, making it (of their todos of before
// lists, if any) when it's missing.
func (p *Plugin) ensurePersonal(userID string) (*TodoList, error) {
	if err := p.migratePersonal(userID); err != nil {
		return nil, err
	}
	now := model.GetMillis()
	return p.ensureList(&TodoList{ID: defaultPersonalListID(userID), Name: DefaultPersonalListName, OwnerID: userID, CreateAt: now, UpdateAt: now})
}

// mergeItems adds the todos that aren't in a list yet at the end of its open ones.
func (p *Plugin) mergeItems(listID string, added []*Item) error {
	_, err := updateKV(p.API, itemsKey(listID), func(items *[]*Item) error {
		var missing []*Item
		for _, item := range added {
			if !slices.ContainsFunc(*items, func(t *Item) bool { return t.ID == item.ID }) {
				missing = append(missing, item)
			}
		}
		if len(missing) > 0 {
			*items = insertOpen(*items, missing...)
		}
		return nil
	})
	return err
}

// retire backs an old key up and deletes it, unless it changed since it was read: then it's
// migrated again.
func (p *Plugin) retire(key string, old []byte) (bool, error) {
	backup := keyBackupPrefix + key + "_" + strconv.FormatInt(model.GetMillis(), 10) + "_" + model.NewId()[:8]
	if appErr := p.API.KVSet(backup, old); appErr != nil {
		return false, appErr
	}
	ok, appErr := p.API.KVCompareAndDelete(key, old)
	if appErr != nil {
		return false, appErr
	}
	return ok, nil
}

// parseOrder reads someone's own list of before lists: references to todos, or (older still) their
// IDs.
func parseOrder(data []byte) ([]*IssueRef, error) {
	var refs []*IssueRef
	if err := json.Unmarshal(data, &refs); err == nil {
		return refs, nil
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, err
	}
	for _, id := range ids {
		refs = append(refs, &IssueRef{IssueID: id})
	}
	return refs, nil
}

// migratePersonal makes someone's default list of their own todos of before lists.
func (p *Plugin) migratePersonal(userID string) error {
	key := StoreListKey + "_" + userID
	for range StoreRetries + 2 {
		old, appErr := p.API.KVGet(key)
		if appErr != nil {
			return appErr
		}
		if old == nil {
			return nil
		}
		refs, err := parseOrder(old)
		if err != nil {
			// Kept as it is: better unseen than lost
			p.API.LogError("Cannot read someone's todos to migrate them", "user_id", userID, "error", err.Error())
			return nil
		}
		now := model.GetMillis()
		list, err := p.ensureList(&TodoList{ID: defaultPersonalListID(userID), Name: DefaultPersonalListName, OwnerID: userID, CreateAt: now, UpdateAt: now})
		if err != nil {
			return err
		}
		items := make([]*Item, 0, len(refs))
		for _, ref := range refs {
			var issue Issue
			found, getErr := kvGetJSON(p.API, issueKey(ref.IssueID), &issue)
			if getErr != nil {
				return getErr
			}
			if !found {
				p.API.LogWarn("A todo to migrate is missing", "user_id", userID, "issue_id", ref.IssueID)
				continue
			}
			if issue.ID == "" {
				issue.ID = ref.IssueID
			}
			item := &Item{
				ID:            issue.ID,
				ListID:        list.ID,
				Message:       issue.Message,
				Description:   issue.Description,
				CreatorID:     userID,
				CreateAt:      issue.CreateAt,
				UpdateAt:      issue.CreateAt,
				PostID:        issue.PostID,
				PostPermalink: issue.PostPermalink,
			}
			if ref.ForeignUserID != "" {
				// Accepted from someone: linked to their copy, which their "Sent by you" shows
				item.CreatorID, item.FromUserID, item.FromIssueID = ref.ForeignUserID, ref.ForeignUserID, ref.ForeignIssueID
				issue.ListID = list.ID
				data, marshalErr := json.Marshal(&issue)
				if marshalErr != nil {
					return marshalErr
				}
				if appErr := p.API.KVSet(issueKey(issue.ID), data); appErr != nil {
					return appErr
				}
			}
			items = append(items, item)
		}
		if err = p.mergeItems(list.ID, items); err != nil {
			return err
		}
		if ok, err := p.retire(key, old); err != nil || ok {
			return err
		}
	}
	return errors.New("someone's todos changed too often to migrate them")
}

func defaultChannelListName(channel *model.Channel) string {
	switch {
	case channel == nil:
		return "Channel todos"
	case isConversation(channel):
		return "Conversation todos"
	case channel.DisplayName != "":
		return channel.DisplayName + " todos"
	default:
		return channel.Name + " todos"
	}
}

// migrateChannel makes a channel's first list of its todos of before lists.
func (p *Plugin) migrateChannel(channelID string) error {
	key := oldChannelListPrefix + channelID
	for range StoreRetries + 2 {
		old, appErr := p.API.KVGet(key)
		if appErr != nil {
			return appErr
		}
		if old == nil {
			return nil
		}
		var todos []*legacyChannelTodo
		if err := json.Unmarshal(old, &todos); err != nil {
			p.API.LogError("Cannot read a channel's todos to migrate them", "channel_id", channelID, "error", err.Error())
			return nil
		}
		channel, appErr := p.API.GetChannel(channelID)
		if appErr != nil {
			channel = nil
		}
		now := model.GetMillis()
		list, err := p.ensureList(&TodoList{ID: defaultChannelListID(channelID), Name: defaultChannelListName(channel), ChannelID: channelID, CreateAt: now, UpdateAt: now})
		if err != nil {
			return err
		}
		items := make([]*Item, 0, len(todos))
		for _, t := range todos {
			items = append(items, &Item{
				ID:            t.ID,
				ListID:        list.ID,
				Message:       t.Message,
				Description:   t.Description,
				CreatorID:     t.CreatorID,
				CreateAt:      t.CreateAt,
				UpdateAt:      t.UpdateAt,
				AssigneeID:    t.AssigneeID,
				DoneBy:        t.DoneBy,
				DoneAt:        t.DoneAt,
				PostID:        t.PostID,
				PostPermalink: t.PostPermalink,
			})
		}
		if err = p.mergeItems(list.ID, items); err != nil {
			return err
		}
		if ok, err := p.retire(key, old); err != nil || ok {
			return err
		}
	}
	return errors.New("a channel's todos changed too often to migrate them")
}

// migrateFollowing makes favorites of the lists of the channels someone followed.
func (p *Plugin) migrateFollowing(userID string) error {
	key := oldFollowingPrefix + userID
	for range StoreRetries + 2 {
		old, appErr := p.API.KVGet(key)
		if appErr != nil {
			return appErr
		}
		if old == nil {
			return nil
		}
		var channelIDs []string
		if err := json.Unmarshal(old, &channelIDs); err != nil {
			p.API.LogError("Cannot read the lists someone follows to migrate them", "user_id", userID, "error", err.Error())
			return nil
		}
		var listIDs []string
		for _, channelID := range channelIDs {
			if err := p.migrateChannel(channelID); err != nil {
				return err
			}
			// A channel's list that had no todos has no list now: nothing to follow
			if list, err := p.getList(defaultChannelListID(channelID)); err == nil && list != nil {
				listIDs = append(listIDs, list.ID)
			}
		}
		if _, err := updateKV(p.API, favoritesKey(userID), func(ids *[]string) error {
			for _, id := range listIDs {
				if !slices.Contains(*ids, id) && len(*ids) < maxFavorites {
					*ids = append(*ids, id)
				}
			}
			return nil
		}); err != nil {
			return err
		}
		if ok, err := p.retire(key, old); err != nil || ok {
			return err
		}
	}
	return errors.New("the lists someone follows changed too often to migrate them")
}

// isOldKey tells whether a key is prefix followed by an ID, and returns the ID.
func isOldKey(key, prefix string) (string, bool) {
	id, found := strings.CutPrefix(key, prefix)
	return id, found && len(id) == 26 && model.IsValidId(id)
}

// MigrateAll migrates everyone's and every channel's todos of before lists, once (on one server of
// a cluster at a time), and records it.
func (p *Plugin) MigrateAll() error {
	var record migrationRecord
	if _, err := kvGetJSON(p.API, keyMigration, &record); err != nil {
		return err
	}
	if record.Version >= ListsDataVersion {
		return nil
	}
	locked, appErr := p.API.KVSetWithOptions(keyMigrationLock, []byte("1"), model.PluginKVSetOptions{Atomic: true, OldValue: nil, ExpireInSeconds: migrationLockSeconds})
	if appErr != nil {
		return appErr
	}
	if !locked {
		return nil
	}
	defer func() {
		if appErr := p.API.KVDelete(keyMigrationLock); appErr != nil {
			p.API.LogWarn("Could not release the migration's lock", "error", appErr.Error())
		}
	}()

	// The keys first: migrating deletes some, which would shift the pages.
	var users, channels, following []string
	for page := 0; ; page++ {
		keys, appErr := p.API.KVList(page, kvListPage)
		if appErr != nil {
			return appErr
		}
		for _, key := range keys {
			if id, ok := isOldKey(key, StoreListKey+"_"); ok {
				users = append(users, id)
			} else if id, ok := isOldKey(key, oldChannelListPrefix); ok {
				channels = append(channels, id)
			} else if id, ok := isOldKey(key, oldFollowingPrefix); ok {
				following = append(following, id)
			}
		}
		if len(keys) < kvListPage {
			break
		}
	}

	failed := 0
	for _, userID := range users {
		if err := p.migratePersonal(userID); err != nil {
			failed++
			p.API.LogError("Could not migrate someone's todos", "user_id", userID, "error", err.Error())
		}
	}
	for _, channelID := range channels {
		if err := p.migrateChannel(channelID); err != nil {
			failed++
			p.API.LogError("Could not migrate a channel's todos", "channel_id", channelID, "error", err.Error())
		}
	}
	for _, userID := range following {
		if err := p.migrateFollowing(userID); err != nil {
			failed++
			p.API.LogError("Could not migrate the lists someone follows", "user_id", userID, "error", err.Error())
		}
	}
	if failed > 0 {
		// Tried again at the next start, and meanwhile when the lists are opened
		return errors.Errorf("%d migrations failed", failed)
	}
	data, err := json.Marshal(migrationRecord{Version: ListsDataVersion, DoneAt: model.GetMillis()})
	if err != nil {
		return err
	}
	if appErr := p.API.KVSet(keyMigration, data); appErr != nil {
		return appErr
	}
	p.API.LogInfo("Migrated the todos to lists", "people", len(users), "channels", len(channels))
	return nil
}
