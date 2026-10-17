// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"net/http"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattermost/mattermost/server/public/model"
)

// Todo lists: a person has personal lists, and a channel (or a conversation) has lists of its own,
// as they have notes. The todos (items) belong to a list. See access.go for who may do what.
//
// The KV store holds:
//
//	tlist_<list>     a list (TodoList)
//	titems_<list>    its todos ([]*Item): the open ones in their order, then the done ones, newest first
//	tlidx_u_<user>   the IDs of someone's personal lists, in their order (their default list first)
//	tlidx_c_<chan>   the IDs of a channel's lists, in their order
//	tlfav_<user>     the IDs of someone's favorite lists, the latest first
//	tllast_<user>    the list someone last added a todo to, per channel ([]lastUsed)
//
// migration.go turns the data of the versions before lists into lists.

const (
	// WSEventListChanged tells those who see a list that it, or its todos, changed.
	WSEventListChanged = "list_changed"

	keyListPrefix         = "tlist_"
	keyItemsPrefix        = "titems_"
	keyUserListsPrefix    = "tlidx_u_"
	keyChannelListsPrefix = "tlidx_c_"
	keyFavoritesPrefix    = "tlfav_"
	keyLastUsedPrefix     = "tllast_"

	// ScopePersonal, ScopeChannel and ScopeConversation tell whose a list is.
	ScopePersonal     = "personal"
	ScopeChannel      = "channel"
	ScopeConversation = "conversation"

	// DefaultPersonalListName is the name of everyone's first personal list, where the todos they
	// accept land.
	DefaultPersonalListName = "My todos"

	maxListsPerScope = 50
	maxFavorites     = 200
	maxLastUsed      = 100
	maxListName      = 100
	maxListIcon      = 8
)

// TodoList is a list of todos: someone's own (no channel), or a channel's.
type TodoList struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Icon is an optional emoji.
	Icon      string `json:"icon,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
	// OwnerID is the owner of a personal list, and who created a channel's list (none for the list
	// made from a channel's todos by the migration).
	OwnerID    string `json:"owner_id"`
	CreateAt   int64  `json:"create_at"`
	UpdateAt   int64  `json:"update_at"`
	ArchivedAt int64  `json:"archived_at,omitempty"`
}

// ListInfo is a list as someone sees it in the list of lists: where it is, what they may do with
// it, and how much is left to do.
type ListInfo struct {
	TodoList
	Scope          string `json:"scope"`
	ChannelName    string `json:"channel_name,omitempty"`
	ChannelURLName string `json:"channel_url_name,omitempty"`
	TeamName       string `json:"team_name,omitempty"`
	// Default: the person's default personal list, which isn't archived or deleted.
	Default   bool `json:"default,omitempty"`
	Favorite  bool `json:"favorite"`
	CanEdit   bool `json:"can_edit"`
	CanRename bool `json:"can_rename"`
	CanDelete bool `json:"can_delete"`
	CanManage bool `json:"can_manage"`
	OpenCount int  `json:"open_count"`
	// EarliestDue is the earliest due date of the open todos (YYYY-MM-DD), for the overdue and due
	// soon marks: the client tells them in its own time zone.
	EarliestDue string `json:"earliest_due,omitempty"`
}

// ListDetail is an open list: its info and its todos.
type ListDetail struct {
	ListInfo
	Items []*Item `json:"items"`
}

type lastUsed struct {
	ChannelID string `json:"channel_id"`
	ListID    string `json:"list_id"`
}

func listKey(listID string) string            { return keyListPrefix + listID }
func itemsKey(listID string) string           { return keyItemsPrefix + listID }
func userListsKey(userID string) string       { return keyUserListsPrefix + userID }
func channelListsKey(channelID string) string { return keyChannelListsPrefix + channelID }
func favoritesKey(userID string) string       { return keyFavoritesPrefix + userID }
func lastUsedKey(userID string) string        { return keyLastUsedPrefix + userID }

// scopeKey is the key of the index of the lists of a list's scope.
func scopeKey(list *TodoList) string {
	if list.ChannelID != "" {
		return channelListsKey(list.ChannelID)
	}
	return userListsKey(list.OwnerID)
}

// Store -------------------------------------------------------------------------------------------

// getList returns a list, or nil when there's none.
func (p *Plugin) getList(listID string) (*TodoList, error) {
	var list TodoList
	found, err := kvGetJSON(p.API, listKey(listID), &list)
	if err != nil || !found {
		return nil, err
	}
	return &list, nil
}

func (p *Plugin) saveList(list *TodoList) error {
	_, err := updateKV(p.API, listKey(list.ID), func(stored *TodoList) error {
		*stored = *list
		return nil
	})
	return err
}

func (p *Plugin) getItems(listID string) ([]*Item, error) {
	items := []*Item{}
	if _, err := kvGetJSON(p.API, itemsKey(listID), &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *Plugin) getIDs(key string) ([]string, error) {
	ids := []string{}
	if _, err := kvGetJSON(p.API, key, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// listsOf returns the lists of an index that still exist.
func (p *Plugin) listsOf(key string) ([]*TodoList, error) {
	ids, err := p.getIDs(key)
	if err != nil {
		return nil, err
	}
	lists := make([]*TodoList, 0, len(ids))
	for _, id := range ids {
		list, err := p.getList(id)
		if err != nil {
			return nil, err
		}
		if list != nil {
			lists = append(lists, list)
		}
	}
	return lists, nil
}

// Info --------------------------------------------------------------------------------------------

func listScope(list *TodoList, channel *model.Channel) string {
	switch {
	case list.ChannelID == "":
		return ScopePersonal
	case isConversation(channel):
		return ScopeConversation
	default:
		return ScopeChannel
	}
}

// summarize counts the open todos of a list, and finds the earliest due date among them.
func summarize(items []*Item) (open int, earliestDue string) {
	for _, item := range items {
		if item.Done() {
			continue
		}
		open++
		if item.Due != "" && (earliestDue == "" || item.Due < earliestDue) {
			earliestDue = item.Due
		}
	}
	return open, earliestDue
}

func (p *Plugin) listInfo(userID string, list *TodoList, rights listRights, items []*Item, favorites []string) ListInfo {
	info := ListInfo{
		TodoList:  *list,
		Scope:     listScope(list, rights.Channel),
		Default:   list.ChannelID == "" && list.ID == defaultPersonalListID(userID),
		Favorite:  slices.Contains(favorites, list.ID),
		CanEdit:   rights.Edit,
		CanRename: rights.Rename,
		CanDelete: rights.Delete,
		CanManage: rights.Manage,
	}
	info.OpenCount, info.EarliestDue = summarize(items)
	if channel := rights.Channel; channel != nil {
		info.ChannelURLName = channel.Name
		if !isConversation(channel) {
			info.ChannelName = channel.DisplayName
		}
		if channel.TeamId != "" {
			if team, appErr := p.API.GetTeam(channel.TeamId); appErr == nil {
				info.TeamName = team.Name
			}
		}
	}
	return info
}

// infos returns what someone sees of lists, leaving out those they can't read.
func (p *Plugin) infos(userID string, lists []*TodoList) ([]ListInfo, error) {
	favorites, err := p.getIDs(favoritesKey(userID))
	if err != nil {
		return nil, err
	}
	result := make([]ListInfo, 0, len(lists))
	for _, list := range lists {
		rights, err := p.listRights(userID, list)
		if err != nil {
			return nil, err
		}
		if !rights.Read {
			continue
		}
		items, err := p.getItems(list.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, p.listInfo(userID, list, rights, items, favorites))
	}
	return result, nil
}

// PersonalLists returns someone's own lists, archived ones included.
func (p *Plugin) PersonalLists(userID string) ([]ListInfo, error) {
	if _, err := p.ensurePersonal(userID); err != nil {
		return nil, err
	}
	lists, err := p.listsOf(userListsKey(userID))
	if err != nil {
		return nil, err
	}
	return p.infos(userID, lists)
}

// ChannelLists returns the lists of a channel someone is a member of, archived ones included.
func (p *Plugin) ChannelLists(userID, channelID string) ([]ListInfo, error) {
	if _, err := p.memberChannel(userID, channelID); err != nil {
		return nil, err
	}
	if err := p.migrateChannel(channelID); err != nil {
		return nil, err
	}
	lists, err := p.listsOf(channelListsKey(channelID))
	if err != nil {
		return nil, err
	}
	return p.infos(userID, lists)
}

// FavoriteLists returns someone's favorite lists, of any scope, that they can still read.
func (p *Plugin) FavoriteLists(userID string) ([]ListInfo, error) {
	if err := p.migrateFollowing(userID); err != nil {
		return nil, err
	}
	lists, err := p.listsOf(favoritesKey(userID))
	if err != nil {
		return nil, err
	}
	return p.infos(userID, lists)
}

// GetList returns a list someone can read, with its todos.
func (p *Plugin) GetList(userID, listID string) (*ListDetail, error) {
	list, rights, err := p.readableList(userID, listID)
	if err != nil {
		return nil, err
	}
	items, err := p.getItems(list.ID)
	if err != nil {
		return nil, err
	}
	favorites, err := p.getIDs(favoritesKey(userID))
	if err != nil {
		return nil, err
	}
	return &ListDetail{ListInfo: p.listInfo(userID, list, rights, items, favorites), Items: items}, nil
}

// Changes -----------------------------------------------------------------------------------------

func validateListName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", newAPIError(http.StatusBadRequest, "a list needs a name")
	}
	if utf8.RuneCountInString(name) > maxListName {
		return "", newAPIError(http.StatusBadRequest, "a list's name is at most %d characters", maxListName)
	}
	return name, nil
}

// validateIcon accepts no icon, or a short emoji: no letters, digits, spaces or control characters.
func validateIcon(icon string) (string, error) {
	icon = strings.TrimSpace(icon)
	if utf8.RuneCountInString(icon) > maxListIcon {
		return "", newAPIError(http.StatusBadRequest, "a list's icon is an emoji")
	}
	for _, r := range icon {
		if r < 0x80 || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", newAPIError(http.StatusBadRequest, "a list's icon is an emoji")
		}
	}
	return icon, nil
}

type listRequest struct {
	Name *string `json:"name"`
	Icon *string `json:"icon"`
	// ChannelID makes a channel's list (when creating one); none makes a personal list.
	ChannelID string `json:"channel_id"`
	Archived  *bool  `json:"archived"`
	// Position moves the list to that index among the lists of its scope.
	Position *int `json:"position"`
}

// CreateList makes a list: a personal one, or one of a channel the person can post in.
func (p *Plugin) CreateList(userID string, req listRequest) (*ListDetail, error) {
	name := ""
	if req.Name != nil {
		name = *req.Name
	}
	name, err := validateListName(name)
	if err != nil {
		return nil, err
	}
	icon := ""
	if req.Icon != nil {
		if icon, err = validateIcon(*req.Icon); err != nil {
			return nil, err
		}
	}
	now := model.GetMillis()
	list := &TodoList{ID: model.NewId(), Name: name, Icon: icon, ChannelID: req.ChannelID, OwnerID: userID, CreateAt: now, UpdateAt: now}
	if req.ChannelID != "" {
		channel, err := p.memberChannel(userID, req.ChannelID)
		if err != nil {
			return nil, err
		}
		if !channel.CanEdit {
			return nil, newAPIError(http.StatusForbidden, "you can't add lists to this channel")
		}
		// The channel's todos of before lists come first
		if err = p.migrateChannel(req.ChannelID); err != nil {
			return nil, err
		}
	} else if _, err = p.ensurePersonal(userID); err != nil {
		return nil, err
	}

	if err = p.saveList(list); err != nil {
		return nil, err
	}
	if _, err = updateKV(p.API, scopeKey(list), func(ids *[]string) error {
		if len(*ids) >= maxListsPerScope {
			return newAPIError(http.StatusBadRequest, "there are already %d lists here", maxListsPerScope)
		}
		*ids = append(*ids, list.ID)
		return nil
	}); err != nil {
		if appErr := p.API.KVDelete(listKey(list.ID)); appErr != nil {
			p.API.LogWarn("Could not remove a list that couldn't be indexed", "error", appErr.Error())
		}
		return nil, err
	}
	p.sendListChanged(list)
	return p.GetList(userID, list.ID)
}

// UpdateList renames a list, changes its icon, archives or unarchives it, or moves it among the
// lists of its scope.
func (p *Plugin) UpdateList(userID, listID string, req listRequest) (*ListDetail, error) {
	list, rights, err := p.readableList(userID, listID)
	if err != nil {
		return nil, err
	}
	if !rights.Rename {
		return nil, newAPIError(http.StatusForbidden, "you can't change this list")
	}
	changed := *list
	if req.Name != nil {
		if changed.Name, err = validateListName(*req.Name); err != nil {
			return nil, err
		}
	}
	if req.Icon != nil {
		if changed.Icon, err = validateIcon(*req.Icon); err != nil {
			return nil, err
		}
	}
	if req.Archived != nil {
		switch {
		case *req.Archived && list.ChannelID == "" && list.ID == defaultPersonalListID(userID):
			return nil, newAPIError(http.StatusBadRequest, "your default list can't be archived")
		case *req.Archived && changed.ArchivedAt == 0:
			changed.ArchivedAt = model.GetMillis()
		case !*req.Archived:
			changed.ArchivedAt = 0
		}
	}
	if changed != *list {
		changed.UpdateAt = model.GetMillis()
		if err = p.saveList(&changed); err != nil {
			return nil, err
		}
	}
	if req.Position != nil {
		if _, err = updateKV(p.API, scopeKey(list), func(ids *[]string) error {
			i := slices.Index(*ids, list.ID)
			if i < 0 {
				return nil
			}
			rest := slices.Delete(slices.Clone(*ids), i, i+1)
			position := min(max(*req.Position, 0), len(rest))
			*ids = slices.Insert(rest, position, list.ID)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	p.sendListChanged(&changed)
	return p.GetList(userID, listID)
}

// DeleteList deletes a list and its todos. Whoever sent a todo of it is told it was declined.
func (p *Plugin) DeleteList(userID, listID string) error {
	list, rights, err := p.readableList(userID, listID)
	if err != nil {
		return err
	}
	if !rights.Delete {
		if list.ChannelID == "" {
			return newAPIError(http.StatusBadRequest, "your default list can't be deleted")
		}
		return newAPIError(http.StatusForbidden, "only the list's creator or a channel admin can delete it")
	}
	items, err := p.getItems(list.ID)
	if err != nil {
		return err
	}
	if _, err = updateKV(p.API, scopeKey(list), func(ids *[]string) error {
		*ids = slices.DeleteFunc(*ids, func(id string) bool { return id == list.ID })
		return nil
	}); err != nil {
		return err
	}
	if appErr := p.API.KVDelete(itemsKey(list.ID)); appErr != nil {
		return appErr
	}
	if appErr := p.API.KVDelete(listKey(list.ID)); appErr != nil {
		return appErr
	}
	for _, item := range items {
		if item.AssigneeID != "" && !item.Done() {
			p.unindexAssigned(item.AssigneeID, item.ID)
		}
		if item.FromIssueID != "" {
			p.releaseRequest(userID, item, requestDeclined)
		}
	}
	// The favorites and last used lists of others forget it when they next look.
	p.sendListChanged(list)
	return nil
}

// SetFavorite adds a list someone can read to their favorites, or removes one.
func (p *Plugin) SetFavorite(userID, listID string, favorite bool) error {
	if favorite {
		if _, _, err := p.readableList(userID, listID); err != nil {
			return err
		}
	}
	if err := p.migrateFollowing(userID); err != nil {
		return err
	}
	_, err := updateKV(p.API, favoritesKey(userID), func(ids *[]string) error {
		i := slices.Index(*ids, listID)
		switch {
		case favorite && i < 0:
			if len(*ids) >= maxFavorites {
				return newAPIError(http.StatusBadRequest, "you have too many favorite lists already")
			}
			*ids = append([]string{listID}, *ids...)
		case !favorite && i >= 0:
			*ids = slices.Delete(*ids, i, i+1)
		}
		return nil
	})
	return err
}

// Where todos go ----------------------------------------------------------------------------------

// rememberLastUsed notes the list someone added a todo to from a channel, to offer it first there.
func (p *Plugin) rememberLastUsed(userID, channelID, listID string) {
	if channelID == "" {
		return
	}
	if _, err := updateKV(p.API, lastUsedKey(userID), func(entries *[]lastUsed) error {
		kept := []lastUsed{{ChannelID: channelID, ListID: listID}}
		for _, e := range *entries {
			if e.ChannelID != channelID && len(kept) < maxLastUsed {
				kept = append(kept, e)
			}
		}
		*entries = kept
		return nil
	}); err != nil {
		p.API.LogWarn("Could not remember the last list used", "error", err.Error())
	}
}

func (p *Plugin) lastUsedList(userID, channelID string) string {
	var entries []lastUsed
	if _, err := kvGetJSON(p.API, lastUsedKey(userID), &entries); err != nil {
		return ""
	}
	for _, e := range entries {
		if e.ChannelID == channelID {
			return e.ListID
		}
	}
	return ""
}

// Targets are the lists someone can add a todo to from a channel, and the one to offer first: the
// list they last added to from that channel, else the channel's first list, else their default
// personal list.
type Targets struct {
	Lists     []ListInfo `json:"lists"`
	DefaultID string     `json:"default_id"`
}

func (p *Plugin) Targets(userID, channelID string) (*Targets, error) {
	var lists []*TodoList
	if channelID != "" {
		if channel, err := p.channelRights(userID, channelID); err == nil && channel.Member {
			if err = p.migrateChannel(channelID); err != nil {
				return nil, err
			}
			channelLists, err := p.listsOf(channelListsKey(channelID))
			if err != nil {
				return nil, err
			}
			lists = append(lists, channelLists...)
		}
	}
	personal, err := p.ensurePersonal(userID)
	if err != nil {
		return nil, err
	}
	personalLists, err := p.listsOf(userListsKey(userID))
	if err != nil {
		return nil, err
	}
	lists = append(lists, personalLists...)

	infos, err := p.infos(userID, lists)
	if err != nil {
		return nil, err
	}
	targets := &Targets{Lists: []ListInfo{}, DefaultID: personal.ID}
	for _, info := range infos {
		if info.CanEdit {
			targets.Lists = append(targets.Lists, info)
		}
	}
	last := p.lastUsedList(userID, channelID)
	if last != "" && !slices.ContainsFunc(targets.Lists, func(info ListInfo) bool { return info.ID == last }) {
		// The last list used from this channel can be another channel's (one of their favorites)
		if list, err := p.getList(last); err == nil && list != nil {
			if more, err := p.infos(userID, []*TodoList{list}); err == nil && len(more) == 1 && more[0].CanEdit {
				targets.Lists = append(targets.Lists, more[0])
			}
		}
	}
	firstOfChannel := ""
	for _, info := range targets.Lists {
		if info.ID == last {
			targets.DefaultID = last
			return targets, nil
		}
		if firstOfChannel == "" && info.ChannelID != "" && info.ChannelID == channelID {
			firstOfChannel = info.ID
		}
	}
	if firstOfChannel != "" {
		targets.DefaultID = firstOfChannel
	}
	return targets, nil
}

// defaultTarget is the list a todo goes to when none is named (see Targets).
func (p *Plugin) defaultTarget(userID, channelID string) (*TodoList, error) {
	targets, err := p.Targets(userID, channelID)
	if err != nil {
		return nil, err
	}
	for _, info := range targets.Lists {
		if info.ID == targets.DefaultID {
			list := info.TodoList
			return &list, nil
		}
	}
	return p.ensurePersonal(userID)
}

func (p *Plugin) sendListChanged(list *TodoList) {
	data := map[string]any{"list_id": list.ID, "channel_id": list.ChannelID}
	if list.ChannelID != "" {
		p.API.PublishWebSocketEvent(WSEventListChanged, data, &model.WebsocketBroadcast{ChannelId: list.ChannelID})
		return
	}
	p.API.PublishWebSocketEvent(WSEventListChanged, data, &model.WebsocketBroadcast{UserId: list.OwnerID})
}
