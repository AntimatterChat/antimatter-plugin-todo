// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"bytes"
	"net/http"
	"sync"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	chanID   = "c0000000000000000000000000"
	roChanID = "r0000000000000000000000000"
	alice    = "a0000000000000000000000000"
	bob      = "b0000000000000000000000000"
	carol    = "k0000000000000000000000000" // a channel admin
	reader   = "e0000000000000000000000000" // may read, not post
	outsider = "o0000000000000000000000000"
)

type channelFixture struct {
	p   *Plugin
	api *plugintest.API

	mu  sync.Mutex
	kv  map[string][]byte
	dms map[string][]string
}

func newChannelFixture(t *testing.T) *channelFixture {
	f := &channelFixture{api: &plugintest.API{}, kv: map[string][]byte{}, dms: map[string][]string{}}
	api := f.api
	api.On("KVGet", mock.Anything).Return(func(key string) []byte {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.kv[key]
	}, nil)
	api.On("KVCompareAndSet", mock.Anything, mock.Anything, mock.Anything).Return(func(key string, old, value []byte) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !bytes.Equal(f.kv[key], old) {
			return false
		}
		f.kv[key] = value
		return true
	}, nil)
	channel := &model.Channel{Id: chanID, Name: "town-square", DisplayName: "Town Square", Type: model.ChannelTypeOpen, TeamId: "t0000000000000000000000000"}
	api.On("GetChannel", chanID).Return(channel, nil)
	api.On("GetChannel", roChanID).Return(&model.Channel{Id: roChanID, Name: "news", Type: model.ChannelTypeOpen}, nil)
	api.On("GetChannel", mock.Anything).Return(nil, model.NewAppError("GetChannel", "not found", nil, "", http.StatusNotFound))
	for _, u := range []string{alice, bob, carol} {
		api.On("HasPermissionToChannel", u, chanID, model.PermissionReadChannel).Return(true)
		api.On("HasPermissionToChannel", u, chanID, model.PermissionCreatePost).Return(true)
		api.On("HasPermissionToChannel", u, chanID, model.PermissionManagePublicChannelProperties).Return(u == carol)
		api.On("GetChannelMember", chanID, u).Return(&model.ChannelMember{ChannelId: chanID, UserId: u}, nil)
		api.On("GetUser", u).Return(&model.User{Id: u, Username: map[string]string{alice: "alice", bob: "bob", carol: "carol"}[u]}, nil)
	}
	api.On("HasPermissionToChannel", reader, chanID, model.PermissionReadChannel).Return(true)
	api.On("HasPermissionToChannel", reader, chanID, model.PermissionCreatePost).Return(false)
	api.On("HasPermissionToChannel", alice, roChanID, model.PermissionReadChannel).Return(true)
	api.On("HasPermissionToChannel", alice, roChanID, model.PermissionCreatePost).Return(false)
	api.On("HasPermissionToChannel", outsider, mock.Anything, mock.Anything).Return(false)
	api.On("GetChannelMember", chanID, mock.Anything).Return(nil, model.NewAppError("GetChannelMember", "not found", nil, "", http.StatusNotFound))
	api.On("GetTeam", mock.Anything).Return(&model.Team{Name: "lumen"}, nil)
	api.On("PublishWebSocketEvent", mock.Anything, mock.Anything, mock.Anything).Return()
	api.On("GetDirectChannel", mock.Anything, mock.Anything).Return(func(userID, _ string) *model.Channel { return &model.Channel{Id: "dm-" + userID} }, nil)
	api.On("CreatePost", mock.Anything).Run(func(args mock.Arguments) {
		post := args.Get(0).(*model.Post)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.dms[post.ChannelId] = append(f.dms[post.ChannelId], post.Message)
	}).Return(&model.Post{}, nil)
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything).Maybe()

	f.p = &Plugin{BotUserID: "bot"}
	f.p.SetAPI(api)
	f.p.listManager = NewListManager(api)
	return f
}

func str(s string) *string { return &s }
func yes() *bool           { b := true; return &b }
func no() *bool            { b := false; return &b }

func (f *channelFixture) list(t *testing.T) []*ChannelTodo {
	todos, err := f.p.getChannelTodos(chanID)
	require.NoError(t, err)
	return todos
}

func (f *channelFixture) assigned(t *testing.T, userID string) []*AssignedTodo {
	todos, err := f.p.AssignedTodos(userID)
	require.NoError(t, err)
	return todos
}

func status(err error) int {
	if a, ok := err.(*apiError); ok {
		return a.status
	}
	return 0
}

func TestChannelTodosAreSharedWithWhoCanPost(t *testing.T) {
	f := newChannelFixture(t)
	first, err := f.p.AddChannelTodo(alice, chanID, channelTodoRequest{Message: str("  Order the bulbs  ")})
	require.NoError(t, err)
	assert.Equal(t, "Order the bulbs", first.Message)
	_, err = f.p.AddChannelTodo(bob, chanID, channelTodoRequest{Message: str("Book the room")})
	require.NoError(t, err)

	todos := f.list(t)
	require.Len(t, todos, 2)
	assert.Equal(t, "Order the bulbs", todos[0].Message, "added at the end of the open todos")

	_, err = f.p.AddChannelTodo(reader, chanID, channelTodoRequest{Message: str("x")})
	assert.Equal(t, http.StatusForbidden, status(err), "a read-only member can't add")
	_, err = f.p.AddChannelTodo(alice, roChanID, channelTodoRequest{Message: str("x")})
	assert.Equal(t, http.StatusForbidden, status(err), "nor anyone in a read-only channel")
	_, err = f.p.AddChannelTodo(outsider, chanID, channelTodoRequest{Message: str("x")})
	assert.Equal(t, http.StatusForbidden, status(err))
	_, err = f.p.AddChannelTodo(alice, chanID, channelTodoRequest{Message: str("   ")})
	assert.Equal(t, http.StatusBadRequest, status(err))

	_, err = f.p.UpdateChannelTodo(reader, chanID, first.ID, channelTodoRequest{Done: yes()})
	assert.Equal(t, http.StatusForbidden, status(err))
}

func TestChannelTodoDoneAndMoved(t *testing.T) {
	f := newChannelFixture(t)
	a, _ := f.p.AddChannelTodo(alice, chanID, channelTodoRequest{Message: str("a")})
	b, _ := f.p.AddChannelTodo(alice, chanID, channelTodoRequest{Message: str("b")})
	c, _ := f.p.AddChannelTodo(alice, chanID, channelTodoRequest{Message: str("c")})

	done, err := f.p.UpdateChannelTodo(bob, chanID, a.ID, channelTodoRequest{Done: yes()})
	require.NoError(t, err)
	assert.Equal(t, bob, done.DoneBy)
	ids := func() (out []string) {
		for _, t := range f.list(t) {
			out = append(out, t.Message)
		}
		return
	}
	assert.Equal(t, []string{"b", "c", "a"}, ids(), "done todos go under the open ones")

	_, err = f.p.UpdateChannelTodo(alice, chanID, c.ID, channelTodoRequest{Position: new(int)})
	require.NoError(t, err)
	assert.Equal(t, []string{"c", "b", "a"}, ids())

	reopened, err := f.p.UpdateChannelTodo(alice, chanID, a.ID, channelTodoRequest{Done: no()})
	require.NoError(t, err)
	assert.Empty(t, reopened.DoneBy)
	assert.Equal(t, []string{"c", "b", "a"}, ids(), "reopened at the end of the open ones")

	edited, err := f.p.UpdateChannelTodo(bob, chanID, b.ID, channelTodoRequest{Message: str("bee"), Description: str("with honey")})
	require.NoError(t, err)
	assert.Equal(t, "with honey", edited.Description)

	_, err = f.p.UpdateChannelTodo(alice, chanID, "z0000000000000000000000000", channelTodoRequest{Done: yes()})
	assert.Equal(t, http.StatusNotFound, status(err))
}

func TestChannelTodoAssignment(t *testing.T) {
	f := newChannelFixture(t)
	todo, err := f.p.AddChannelTodo(alice, chanID, channelTodoRequest{Message: str("Ship 2.4"), AssigneeID: str(bob)})
	require.NoError(t, err)
	assigned := f.assigned(t, bob)
	require.Len(t, assigned, 1)
	assert.Equal(t, "Town Square", assigned[0].ChannelName)
	assert.Equal(t, "lumen", assigned[0].TeamName)
	require.Len(t, f.dms["dm-"+bob], 1)
	assert.Contains(t, f.dms["dm-"+bob][0], "@alice assigned you a todo in ~town-square")

	_, err = f.p.UpdateChannelTodo(alice, chanID, todo.ID, channelTodoRequest{AssigneeID: str(outsider)})
	assert.Equal(t, http.StatusBadRequest, status(err), "only members can be assigned")

	_, err = f.p.UpdateChannelTodo(bob, chanID, todo.ID, channelTodoRequest{Done: yes()})
	require.NoError(t, err)
	assert.Empty(t, f.assigned(t, bob), "done todos leave the assignee's list")

	_, err = f.p.UpdateChannelTodo(bob, chanID, todo.ID, channelTodoRequest{Done: no()})
	require.NoError(t, err)
	assert.Len(t, f.assigned(t, bob), 1, "and come back when reopened")
	assert.Len(t, f.dms["dm-"+bob], 1, "without telling the assignee again")

	_, err = f.p.UpdateChannelTodo(alice, chanID, todo.ID, channelTodoRequest{AssigneeID: str(carol)})
	require.NoError(t, err)
	assert.Empty(t, f.assigned(t, bob))
	assert.Len(t, f.assigned(t, carol), 1)

	_, err = f.p.UpdateChannelTodo(carol, chanID, todo.ID, channelTodoRequest{AssigneeID: str(carol)})
	require.NoError(t, err)
	assert.Empty(t, f.dms["dm-"+carol][1:], "nobody is told about assigning themselves")
}

func TestChannelTodoDeletion(t *testing.T) {
	f := newChannelFixture(t)
	mine, _ := f.p.AddChannelTodo(alice, chanID, channelTodoRequest{Message: str("mine"), AssigneeID: str(bob)})
	other, _ := f.p.AddChannelTodo(alice, chanID, channelTodoRequest{Message: str("other")})

	err := f.p.DeleteChannelTodo(bob, chanID, mine.ID)
	assert.Equal(t, http.StatusForbidden, status(err), "only its author or a channel admin")
	require.NoError(t, f.p.DeleteChannelTodo(alice, chanID, mine.ID))
	assert.Empty(t, f.assigned(t, bob))
	require.NoError(t, f.p.DeleteChannelTodo(carol, chanID, other.ID))
	assert.Empty(t, f.list(t))
}

func TestAssignedTodosPruneStaleEntries(t *testing.T) {
	f := newChannelFixture(t)
	todo, _ := f.p.AddChannelTodo(alice, chanID, channelTodoRequest{Message: str("x"), AssigneeID: str(bob)})
	// The list lost the todo without the index knowing (e.g. an older version of the plugin).
	f.kv[channelListKey(chanID)] = []byte("[]")
	assert.Empty(t, f.assigned(t, bob))
	assert.JSONEq(t, "[]", string(f.kv[assignedKey(bob)]))
	_ = todo
}

func TestSortChannelTodosKeepsTheLastDone(t *testing.T) {
	var todos []*ChannelTodo
	for i := 0; i < maxDoneChannelTodos+5; i++ {
		todos = append(todos, &ChannelTodo{ID: model.NewId(), DoneAt: int64(i + 1)})
	}
	todos = append(todos, &ChannelTodo{ID: "open"})
	sorted := sortChannelTodos(todos)
	require.Len(t, sorted, maxDoneChannelTodos+1)
	assert.Equal(t, "open", sorted[0].ID)
	assert.EqualValues(t, maxDoneChannelTodos+5, sorted[1].DoneAt, "newest done first")
}
