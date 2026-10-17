// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// Who may do what with a list:
//
//   - A personal list is its owner's alone: nobody else reads it, or learns that it exists.
//   - A list of a channel (or of a conversation: a direct or group message) belongs to the channel:
//     its members read it, and those who can post in the channel change its todos, rename, archive
//     and unarchive it. People who aren't members don't see it, whatever their role (guests
//     included); in an archived or read-only channel, the members only read it.
//   - A channel's list is deleted by whoever created it or a channel admin; in a conversation,
//     which has no admins, by whoever created it (by anyone in it, for the list made from the
//     conversation's todos by the migration, which has no creator).
//   - A todo of a channel's list is deleted by its author or a channel admin.
//   - Only a member of the channel can be assigned a todo of its lists. In a personal list, a todo
//     is sent to someone instead: it leaves the list, and they get it as a request (see
//     requests.go), as before lists existed.

// channelRights are what someone may do in a channel.
type channelRights struct {
	Channel *model.Channel
	// Member: they're in the channel, so they read its lists.
	Member bool
	// CanEdit: they can post in it, and it isn't archived.
	CanEdit bool
	// CanManage: they can also delete the todos and lists of others.
	CanManage bool
}

func isConversation(channel *model.Channel) bool {
	return channel != nil && (channel.Type == model.ChannelTypeDirect || channel.Type == model.ChannelTypeGroup)
}

func (p *Plugin) channelRights(userID, channelID string) (*channelRights, error) {
	channel, appErr := p.API.GetChannel(channelID)
	if appErr != nil {
		return nil, newAPIError(http.StatusNotFound, "channel not found")
	}
	rights := &channelRights{Channel: channel}
	if _, appErr = p.API.GetChannelMember(channelID, userID); appErr != nil {
		if appErr.StatusCode == http.StatusNotFound {
			return rights, nil
		}
		return nil, appErr
	}
	rights.Member = true
	rights.CanEdit = channel.DeleteAt == 0 && p.API.HasPermissionToChannel(userID, channelID, model.PermissionCreatePost)
	rights.CanManage = rights.CanEdit && p.canManageChannel(userID, channel)
	return rights, nil
}

// memberChannel returns a channel the person is a member of.
func (p *Plugin) memberChannel(userID, channelID string) (*channelRights, error) {
	rights, err := p.channelRights(userID, channelID)
	if err != nil {
		return nil, err
	}
	if !rights.Member {
		return nil, newAPIError(http.StatusForbidden, "you can't see this channel")
	}
	return rights, nil
}

func (p *Plugin) canManageChannel(userID string, channel *model.Channel) bool {
	switch channel.Type {
	case model.ChannelTypeOpen:
		return p.API.HasPermissionToChannel(userID, channel.Id, model.PermissionManagePublicChannelProperties)
	case model.ChannelTypePrivate:
		return p.API.HasPermissionToChannel(userID, channel.Id, model.PermissionManagePrivateChannelProperties)
	default:
		// Nobody manages direct and group messages: their todos are deleted by their creators.
		return false
	}
}

// listRights are what someone may do with a list.
type listRights struct {
	// Channel is the list's channel, for a channel's list.
	Channel *model.Channel
	Read    bool
	// Edit: add, change, check off and assign its todos.
	Edit bool
	// Rename: change its name and icon, archive or unarchive it.
	Rename bool
	Delete bool
	// Manage: delete the todos of others.
	Manage bool
}

func (p *Plugin) listRights(userID string, list *TodoList) (listRights, error) {
	if list.ChannelID == "" {
		if list.OwnerID != userID {
			return listRights{}, nil
		}
		return listRights{Read: true, Edit: list.ArchivedAt == 0, Rename: true, Delete: list.ID != defaultPersonalListID(userID), Manage: true}, nil
	}
	channel, err := p.channelRights(userID, list.ChannelID)
	if err != nil {
		if errorStatus(err) == http.StatusNotFound {
			return listRights{}, nil
		}
		return listRights{}, err
	}
	if !channel.Member {
		return listRights{}, nil
	}
	creator := list.OwnerID == userID || (list.OwnerID == "" && isConversation(channel.Channel))
	return listRights{
		Channel: channel.Channel,
		Read:    true,
		Edit:    channel.CanEdit && list.ArchivedAt == 0,
		Rename:  channel.CanEdit,
		Delete:  channel.CanEdit && (creator || channel.CanManage),
		Manage:  channel.CanManage,
	}, nil
}

// readableList returns a list the person may read. A list they may not read is "not found", so
// that nobody learns of the lists of others.
func (p *Plugin) readableList(userID, listID string) (*TodoList, listRights, error) {
	list, err := p.getList(listID)
	if err != nil {
		return nil, listRights{}, err
	}
	if list == nil {
		return nil, listRights{}, newAPIError(http.StatusNotFound, "list not found")
	}
	rights, err := p.listRights(userID, list)
	if err != nil {
		return nil, listRights{}, err
	}
	if !rights.Read {
		return nil, listRights{}, newAPIError(http.StatusNotFound, "list not found")
	}
	return list, rights, nil
}

// readablePost returns a message the person can read, with its permalink: a todo is only linked to
// a message they can see, and the bot only answers in threads they can read.
func (p *Plugin) readablePost(userID, postID string) (*model.Post, string, error) {
	post, appErr := p.API.GetPost(postID)
	if appErr != nil || post.DeleteAt != 0 {
		return nil, "", newAPIError(http.StatusBadRequest, "message not found")
	}
	channel, err := p.channelRights(userID, post.ChannelId)
	if err != nil || !channel.Member {
		return nil, "", newAPIError(http.StatusBadRequest, "message not found")
	}
	return post, p.permalink(channel.Channel, post.Id), nil
}

func (p *Plugin) permalink(channel *model.Channel, postID string) string {
	siteURL := ""
	if config := p.API.GetConfig(); config != nil && config.ServiceSettings.SiteURL != nil {
		siteURL = strings.TrimSuffix(*config.ServiceSettings.SiteURL, "/")
	}
	if channel.TeamId != "" {
		if team, appErr := p.API.GetTeam(channel.TeamId); appErr == nil {
			return fmt.Sprintf("%s/%s/pl/%s", siteURL, team.Name, postID)
		}
	}
	return fmt.Sprintf("%s/_redirect/pl/%s", siteURL, postID)
}
