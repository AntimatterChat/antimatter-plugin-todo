// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFollowingChannelLists(t *testing.T) {
	f := newChannelFixture(t)
	open, err := f.p.AddChannelTodo(bob, chanID, channelTodoRequest{Message: str("Order the bulbs")})
	require.NoError(t, err)
	done, err := f.p.AddChannelTodo(bob, chanID, channelTodoRequest{Message: str("Book the room")})
	require.NoError(t, err)
	_, err = f.p.UpdateChannelTodo(bob, chanID, done.ID, channelTodoRequest{Done: yes()})
	require.NoError(t, err)

	// Only a list one can see is followed
	assert.Equal(t, http.StatusForbidden, status(f.p.SetFollowing(outsider, chanID, true)))

	require.NoError(t, f.p.SetFollowing(alice, chanID, true))
	require.NoError(t, f.p.SetFollowing(alice, chanID, true))
	assert.True(t, f.p.isFollowing(alice, chanID))
	assert.False(t, f.p.isFollowing(bob, chanID))

	lists, err := f.p.FollowedLists(alice)
	require.NoError(t, err)
	require.Len(t, lists, 1, "following twice follows once")
	assert.Equal(t, "Town Square", lists[0].ChannelName)
	assert.Equal(t, "lumen", lists[0].TeamName)
	assert.True(t, lists[0].CanEdit)
	require.Len(t, lists[0].Todos, 1, "the open todos only")
	assert.Equal(t, open.ID, lists[0].Todos[0].ID)

	require.NoError(t, f.p.SetFollowing(alice, chanID, false))
	lists, err = f.p.FollowedLists(alice)
	require.NoError(t, err)
	assert.Empty(t, lists)
}
