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
//   - A channel's list is deleted by whoever created it or its admins: the channel's admins, the
//     community's admins and the system admins. Renaming a channel isn't enough: by default every
//     member may. In a conversation, which has no admins, a list is deleted by whoever created it
//     (by anyone in it, for the list made from the conversation's todos by the migration, which
//     has no creator).
//   - A todo of a channel's list is deleted by its author or those admins. The other members still
//     add, change and check off its todos.
//   - Only a member of the channel can be assigned a todo of its lists. In a personal list, a todo
//     is sent to someone instead: it leaves the list, and they get it as a request (see
//     requests.go), as before lists existed.
//   - A guest sends todos only to people they share a channel with, the people they can see.

// channelRights are what someone may do in a channel.
type channelRights struct {
	Channel *model.Channel
	// Member: they're in the channel, so they read its lists.
	Member bool
	// CanEdit: they can post in it, and it isn't archived.
	CanEdit bool
	// CanManage: they can also delete the todos and lists of others, as an admin of the channel.
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
	member, appErr := p.API.GetChannelMember(channelID, userID)
	if appErr != nil {
		if appErr.StatusCode == http.StatusNotFound {
			return rights, nil
		}
		return nil, appErr
	}
	rights.Member = true
	rights.CanEdit = channel.DeleteAt == 0 && p.API.HasPermissionToChannel(userID, channelID, model.PermissionCreatePost)
	rights.CanManage = rights.CanEdit && p.isChannelAdmin(userID, channel, member)
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

// isChannelAdmin tells whether a member of a channel is one of its admins: a channel admin, an
// admin of its community (who may manage the community) or a system admin. The permissions to
// rename a channel don't count: every member has them by default.
func (p *Plugin) isChannelAdmin(userID string, channel *model.Channel, member *model.ChannelMember) bool {
	if isConversation(channel) {
		// Nobody manages direct and group messages: their todos are deleted by their creators.
		return false
	}
	if member != nil && member.SchemeAdmin {
		return true
	}
	if channel.TeamId != "" && p.API.HasPermissionToTeam(userID, channel.TeamId, model.PermissionManageTeam) {
		return true
	}
	return p.API.HasPermissionTo(userID, model.PermissionManageSystem)
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

// canSendTo tells whether someone may send a todo to someone else: anyone may, except guests, who
// may only send them to people they share a channel with (the people Antimatter lets them see).
// Anyone else is "not found" to a guest, as they are in the people picker.
func (p *Plugin) canSendTo(senderID, receiverID string) error {
	sender, appErr := p.API.GetUser(senderID)
	if appErr != nil {
		return appErr
	}
	if !sender.IsGuest() || senderID == receiverID {
		return nil
	}
	teams, appErr := p.API.GetTeamsForUser(senderID)
	if appErr != nil {
		return appErr
	}
	// The team's channels include the direct and group messages.
	for _, team := range teams {
		mine, appErr := p.API.GetChannelsForTeamForUser(team.Id, senderID, false)
		if appErr != nil {
			return appErr
		}
		theirs, appErr := p.API.GetChannelsForTeamForUser(team.Id, receiverID, false)
		if appErr != nil {
			continue
		}
		shared := make(map[string]bool, len(mine))
		for _, channel := range mine {
			shared[channel.Id] = true
		}
		for _, channel := range theirs {
			if shared[channel.Id] {
				return nil
			}
		}
	}
	return newAPIError(http.StatusNotFound, "user not found")
}
