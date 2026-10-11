// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/gorilla/mux"
)

const (
	storeFollowingKey = "chfollowing"

	// maxFollowed caps the channels whose lists someone follows.
	maxFollowed = 50
)

func followingKey(userID string) string { return storeFollowingKey + "_" + userID }

// FollowedList is the list of a channel someone follows: its open todos, newest first.
type FollowedList struct {
	ChannelID      string `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	ChannelURLName string `json:"channel_url_name"`
	TeamName       string `json:"team_name,omitempty"`
	ChannelPermissions
	Todos []*ChannelTodo `json:"todos"`
}

func (p *Plugin) followedChannels(userID string) ([]string, error) {
	data, appErr := p.API.KVGet(followingKey(userID))
	if appErr != nil {
		return nil, appErr
	}
	var ids []string
	if data != nil {
		if err := json.Unmarshal(data, &ids); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// isFollowing tells whether someone follows a channel's list.
func (p *Plugin) isFollowing(userID, channelID string) bool {
	ids, err := p.followedChannels(userID)
	return err == nil && slices.Contains(ids, channelID)
}

// SetFollowing follows a channel's list (one they can see), or stops following it.
func (p *Plugin) SetFollowing(userID, channelID string, follow bool) error {
	if follow {
		if _, err := p.readableChannel(userID, channelID); err != nil {
			return err
		}
	}
	_, err := updateKV(p, followingKey(userID), func(ids *[]string) error {
		i := slices.Index(*ids, channelID)
		switch {
		case follow && i < 0:
			if len(*ids) >= maxFollowed {
				return newAPIError(http.StatusBadRequest, "you follow too many lists already")
			}
			*ids = append(*ids, channelID)
		case !follow && i >= 0:
			*ids = slices.Delete(*ids, i, i+1)
		}
		return nil
	})
	return err
}

// FollowedLists gives the lists someone follows, of the channels they can still see.
func (p *Plugin) FollowedLists(userID string) ([]*FollowedList, error) {
	ids, err := p.followedChannels(userID)
	if err != nil {
		return nil, err
	}
	lists := []*FollowedList{}
	for _, channelID := range ids {
		channel, err := p.readableChannel(userID, channelID)
		if err != nil {
			continue
		}
		todos, err := p.getChannelTodos(channelID)
		if err != nil {
			return nil, err
		}
		open := []*ChannelTodo{}
		for _, todo := range todos {
			if !todo.Done() {
				open = append(open, todo)
			}
		}
		list := &FollowedList{ChannelID: channelID, ChannelName: channel.DisplayName, ChannelURLName: channel.Name, ChannelPermissions: p.channelPermissions(userID, channel), Todos: open}
		if channel.TeamId != "" {
			if team, appErr := p.API.GetTeam(channel.TeamId); appErr == nil {
				list.TeamName = team.Name
			}
		}
		lists = append(lists, list)
	}
	return lists, nil
}

func (p *Plugin) handleFollowing(w http.ResponseWriter, r *http.Request) {
	lists, err := p.FollowedLists(r.Header.Get("Mattermost-User-ID"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

func (p *Plugin) handleFollow(w http.ResponseWriter, r *http.Request) {
	follow := r.Method == http.MethodPut
	if err := p.SetFollowing(r.Header.Get("Mattermost-User-ID"), mux.Vars(r)["channel_id"], follow); err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"following": follow})
}
