// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/require"
)

// The people and places of the tests.
const (
	teamID = "t0000000000000000000000000"

	town     = "c0000000000000000000000000" // an open channel
	news     = "r0000000000000000000000000" // its members only read it
	attic    = "x0000000000000000000000000" // archived
	dmAB     = "d0000000000000000000000000" // alice and bob's direct messages
	elsewher = "w0000000000000000000000000" // a channel of others

	alice    = "a0000000000000000000000000"
	bob      = "b0000000000000000000000000"
	carol    = "k0000000000000000000000000" // town's admin
	dana     = "n0000000000000000000000000" // in town, an admin of the community
	sam      = "s0000000000000000000000000" // in town, a system admin
	reader   = "e0000000000000000000000000" // in news, where only admins post
	guest    = "g0000000000000000000000000" // a guest, in town
	visitor  = "v0000000000000000000000000" // a guest in no channel
	outsider = "o0000000000000000000000000" // in no channel
)

// fakeAPI is the plugin API over an in-memory KV store and a few channels and people. Calls it
// doesn't fake fail the test (plugintest's mock).
type fakeAPI struct {
	*plugintest.API

	mu       sync.Mutex
	kv       map[string][]byte
	channels map[string]*model.Channel
	members  map[string][]string
	readOnly map[string][]string
	admins   map[string][]string // channel admins, by channel
	// teamAdmins may manage the community; systemAdmins the server.
	teamAdmins   []string
	systemAdmins []string
	users        map[string]*model.User
	posts        map[string]*model.Post

	dms       map[string][]string // bot messages, by person
	replies   []*model.Post       // bot answers in threads
	ephemeral map[string][]string // command answers, by person
	events    []string

	// beforeCompareAndDelete runs before a compare-and-delete, to change the store meanwhile.
	beforeCompareAndDelete func(key string)
}

func newFakeAPI() *fakeAPI {
	f := &fakeAPI{
		API: &plugintest.API{},
		kv:  map[string][]byte{},
		channels: map[string]*model.Channel{
			town:     {Id: town, Name: "town-square", DisplayName: "Town Square", Type: model.ChannelTypeOpen, TeamId: teamID},
			news:     {Id: news, Name: "news", DisplayName: "News", Type: model.ChannelTypeOpen, TeamId: teamID},
			attic:    {Id: attic, Name: "attic", DisplayName: "Attic", Type: model.ChannelTypePrivate, TeamId: teamID, DeleteAt: 1},
			dmAB:     {Id: dmAB, Name: alice + "__" + bob, Type: model.ChannelTypeDirect},
			elsewher: {Id: elsewher, Name: "elsewhere", DisplayName: "Elsewhere", Type: model.ChannelTypePrivate, TeamId: teamID},
		},
		members: map[string][]string{
			town:     {alice, bob, carol, guest, dana, sam},
			news:     {alice, reader},
			attic:    {alice, bob},
			dmAB:     {alice, bob},
			elsewher: {carol},
		},
		readOnly: map[string][]string{news: {reader}},
		admins:   map[string][]string{town: {carol}},

		teamAdmins:   []string{dana},
		systemAdmins: []string{sam},
		users:        map[string]*model.User{},
		posts:        map[string]*model.Post{},
		dms:          map[string][]string{},

		ephemeral: map[string][]string{},
	}
	for id, name := range map[string]string{alice: "alice", bob: "bob", carol: "carol", dana: "dana", sam: "sam", reader: "reader", guest: "gus", visitor: "vic", outsider: "otto"} {
		f.users[id] = &model.User{Id: id, Username: name}
	}
	f.users[guest].Roles = model.SystemGuestRoleId
	f.users[visitor].Roles = model.SystemGuestRoleId
	f.posts["p0000000000000000000000000"] = &model.Post{Id: "p0000000000000000000000000", ChannelId: town, Message: "Order the bulbs"}
	f.posts["q0000000000000000000000000"] = &model.Post{Id: "q0000000000000000000000000", ChannelId: elsewher, Message: "A secret"}
	return f
}

func (f *fakeAPI) isMember(channelID, userID string) bool {
	return slices.Contains(f.members[channelID], userID)
}

// leave takes someone out of a channel.
func (f *fakeAPI) leave(channelID, userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[channelID] = slices.DeleteFunc(slices.Clone(f.members[channelID]), func(id string) bool { return id == userID })
}

// The KV store

func (f *fakeAPI) KVGet(key string) ([]byte, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.kv[key], nil
}

func (f *fakeAPI) KVSet(key string, value []byte) *model.AppError {
	f.mu.Lock()
	defer f.mu.Unlock()
	if value == nil {
		delete(f.kv, key)
	} else {
		f.kv[key] = value
	}
	return nil
}

func (f *fakeAPI) KVDelete(key string) *model.AppError {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.kv, key)
	return nil
}

func (f *fakeAPI) KVCompareAndSet(key string, old, value []byte) (bool, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	current, exists := f.kv[key]
	if (old == nil && exists) || (old != nil && !bytes.Equal(current, old)) {
		return false, nil
	}
	f.kv[key] = value
	return true, nil
}

func (f *fakeAPI) KVCompareAndDelete(key string, old []byte) (bool, *model.AppError) {
	if hook := f.beforeCompareAndDelete; hook != nil {
		hook(key)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !bytes.Equal(f.kv[key], old) {
		return false, nil
	}
	delete(f.kv, key)
	return true, nil
}

func (f *fakeAPI) KVSetWithOptions(key string, value []byte, options model.PluginKVSetOptions) (bool, *model.AppError) {
	if options.Atomic {
		return f.KVCompareAndSet(key, options.OldValue, value)
	}
	return true, f.KVSet(key, value)
}

func (f *fakeAPI) KVList(page, perPage int) ([]string, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.kv))
	for key := range f.kv {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	start := min(page*perPage, len(keys))
	return keys[start:min(start+perPage, len(keys))], nil
}

// keys returns the keys of the store with a prefix.
func (f *fakeAPI) keys(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keys []string
	for key := range f.kv {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func (f *fakeAPI) setJSON(t *testing.T, key string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	require.Nil(t, f.KVSet(key, data))
}

// Channels, people and messages

func (f *fakeAPI) GetChannel(channelID string) (*model.Channel, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if channel, ok := f.channels[channelID]; ok {
		return channel, nil
	}
	return nil, model.NewAppError("GetChannel", "not found", nil, "", http.StatusNotFound)
}

func (f *fakeAPI) GetChannelMember(channelID, userID string) (*model.ChannelMember, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.isMember(channelID, userID) {
		admin := slices.Contains(f.admins[channelID], userID)
		return &model.ChannelMember{ChannelId: channelID, UserId: userID, SchemeUser: true, SchemeAdmin: admin}, nil
	}
	return nil, model.NewAppError("GetChannelMember", "not found", nil, "", http.StatusNotFound)
}

func (f *fakeAPI) HasPermissionToChannel(userID, channelID string, permission *model.Permission) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.isMember(channelID, userID) {
		return false
	}
	switch permission.Id {
	case model.PermissionReadChannel.Id:
		return true
	case model.PermissionCreatePost.Id:
		return !slices.Contains(f.readOnly[channelID], userID)
	case model.PermissionManagePublicChannelProperties.Id, model.PermissionManagePrivateChannelProperties.Id:
		// As on a new server: every member may rename the channel.
		return true
	}
	return false
}

func (f *fakeAPI) HasPermissionToTeam(userID, team string, permission *model.Permission) bool {
	if permission.Id == model.PermissionManageTeam.Id && team == teamID {
		return slices.Contains(f.teamAdmins, userID) || slices.Contains(f.systemAdmins, userID)
	}
	return slices.Contains(f.systemAdmins, userID)
}

func (f *fakeAPI) HasPermissionTo(userID string, _ *model.Permission) bool {
	return slices.Contains(f.systemAdmins, userID)
}

func (f *fakeAPI) GetTeamsForUser(string) ([]*model.Team, *model.AppError) {
	return []*model.Team{{Id: teamID, Name: "lumen"}}, nil
}

// GetChannelsForTeamForUser returns someone's channels of a team, with their direct and group
// messages, as the server does.
func (f *fakeAPI) GetChannelsForTeamForUser(team, userID string, _ bool) ([]*model.Channel, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var channels []*model.Channel
	for id, channel := range f.channels {
		if f.isMember(id, userID) && (channel.TeamId == team || channel.TeamId == "") {
			channels = append(channels, channel)
		}
	}
	return channels, nil
}

func (f *fakeAPI) GetUser(userID string) (*model.User, *model.AppError) {
	if user, ok := f.users[userID]; ok {
		return user, nil
	}
	return nil, model.NewAppError("GetUser", "not found", nil, "", http.StatusNotFound)
}

func (f *fakeAPI) GetUserByUsername(name string) (*model.User, *model.AppError) {
	for _, user := range f.users {
		if user.Username == name {
			return user, nil
		}
	}
	return nil, model.NewAppError("GetUserByUsername", "not found", nil, "", http.StatusNotFound)
}

func (f *fakeAPI) GetTeam(string) (*model.Team, *model.AppError) {
	return &model.Team{Id: teamID, Name: "lumen"}, nil
}

func (f *fakeAPI) GetPost(postID string) (*model.Post, *model.AppError) {
	if post, ok := f.posts[postID]; ok {
		return post, nil
	}
	return nil, model.NewAppError("GetPost", "not found", nil, "", http.StatusNotFound)
}

func (f *fakeAPI) GetConfig() *model.Config {
	config := &model.Config{}
	config.ServiceSettings.SiteURL = model.NewPointer("http://chat.example")
	return config
}

func (f *fakeAPI) GetDirectChannel(userID, _ string) (*model.Channel, *model.AppError) {
	return &model.Channel{Id: "dm-" + userID}, nil
}

func (f *fakeAPI) CreatePost(post *model.Post) (*model.Post, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if user, ok := strings.CutPrefix(post.ChannelId, "dm-"); ok {
		f.dms[user] = append(f.dms[user], post.Message)
	} else {
		f.replies = append(f.replies, post)
	}
	return post, nil
}

func (f *fakeAPI) SendEphemeralPost(userID string, post *model.Post) *model.Post {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ephemeral[userID] = append(f.ephemeral[userID], post.Message)
	return post
}

func (f *fakeAPI) PublishWebSocketEvent(event string, _ map[string]any, _ *model.WebsocketBroadcast) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
}

func (f *fakeAPI) LogDebug(string, ...any) {}
func (f *fakeAPI) LogInfo(string, ...any)  {}
func (f *fakeAPI) LogWarn(string, ...any)  {}
func (f *fakeAPI) LogError(string, ...any) {}

// The plugin

func newTestPlugin(t *testing.T) (*Plugin, *fakeAPI) {
	t.Helper()
	api := newFakeAPI()
	p := &Plugin{BotUserID: "bot"}
	p.SetAPI(api)
	p.listManager = NewListManager(api, p)
	p.initializeAPI()
	return p, api
}

func str(s string) *string { return &s }
func yes() *bool           { b := true; return &b }
func no() *bool            { b := false; return &b }
func at(i int) *int        { return &i }

// status is the HTTP status of an error, or 0 without one.
func status(err error) int {
	if err == nil {
		return 0
	}
	return errorStatus(err)
}

// newList makes a list, failing the test if it can't.
func newList(t *testing.T, p *Plugin, userID, channelID, name string) *ListDetail {
	t.Helper()
	list, err := p.CreateList(userID, listRequest{Name: str(name), ChannelID: channelID})
	require.NoError(t, err)
	return list
}

func addItem(t *testing.T, p *Plugin, userID, listID, message string) *Item {
	t.Helper()
	item, err := p.AddItem(userID, listID, itemRequest{Message: str(message)})
	require.NoError(t, err)
	return item
}

// messages returns the texts of a list's todos, in their order.
func messages(t *testing.T, p *Plugin, listID string) []string {
	t.Helper()
	items, err := p.getItems(listID)
	require.NoError(t, err)
	out := []string{}
	for _, item := range items {
		out = append(out, item.Message)
	}
	return out
}

// call sends a request to the plugin's HTTP API as someone (nobody, with an empty userID).
func call(t *testing.T, p *Plugin, userID, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		require.NoError(t, err)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	if userID != "" {
		r.Header.Set("Mattermost-User-ID", userID)
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(nil, w, r)
	return w
}
