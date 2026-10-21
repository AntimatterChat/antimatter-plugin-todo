package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// Issue represents a Todo issue
type Issue struct {
	ID            string `json:"id"`
	Message       string `json:"message"`
	PostPermalink string `json:"postPermalink"`
	Description   string `json:"description,omitempty"`
	CreateAt      int64  `json:"create_at"`
	PostID        string `json:"post_id"`
	// ListID is, for the receiver's copy of a todo they accepted, the list it's in now.
	ListID string `json:"list_id,omitempty"`
}

// ExtendedIssue extends the information on Issue to be used on the front-end
type ExtendedIssue struct {
	Issue
	ForeignUser     string `json:"user"`
	ForeignUserID   string `json:"user_id,omitempty"`
	ForeignList     string `json:"list"`
	ForeignPosition int    `json:"position"`
}

// ListsIssue for all list issues
type ListsIssue struct {
	In  []*ExtendedIssue `json:"in"`
	My  []*ExtendedIssue `json:"my"`
	Out []*ExtendedIssue `json:"out"`
}

func newIssue(message, postPermalink, description, postID string) *Issue {
	return &Issue{
		ID:            model.NewId(),
		CreateAt:      model.GetMillis(),
		Message:       message,
		PostPermalink: postPermalink,
		Description:   description,
		PostID:        postID,
	}
}

// nothingToDo is what the bot says of an empty list.
const nothingToDo = "Nothing to do!"

func issuesListToString(issues []*ExtendedIssue) string {
	if len(issues) == 0 {
		return nothingToDo
	}

	str := "\n\n"

	for _, issue := range issues {
		createAt := time.Unix(issue.CreateAt/1000, 0)
		str += fmt.Sprintf("* %s\n  * (%s)\n", issue.Message, createAt.Format("January 2, 2006 at 15:04"))
	}

	return str
}

// dueToString tells when a todo is due, from today (YYYY-MM-DD, in the reader's time zone).
func dueToString(due, today string) string {
	switch {
	case due == "":
		return ""
	case due < today:
		return ", overdue since " + due
	case due == today:
		return ", due today"
	default:
		return ", due " + due
	}
}

// itemsToString lists todos, for the bot's messages.
func itemsToString(items []*Item, today string) string {
	if len(items) == 0 {
		return nothingToDo
	}
	str := ""
	for _, item := range items {
		check := "[ ]"
		if item.Done() {
			check = "[x]"
		}
		if due := strings.TrimPrefix(dueToString(item.Due, today), ", "); due != "" && !item.Done() {
			str += fmt.Sprintf("- %s %s (%s)\n", check, item.Message, due)
		} else {
			str += fmt.Sprintf("- %s %s\n", check, item.Message)
		}
	}
	return str
}

// personalListsToString lists the open todos of someone's personal lists, by list, for the daily
// reminder.
func personalListsToString(lists []*TodoList, open map[string][]*Item, today string) string {
	str := ""
	for _, list := range lists {
		items := open[list.ID]
		if len(items) == 0 {
			continue
		}
		str += fmt.Sprintf("**%s**\n", listTitle(list))
		for _, item := range items {
			str += fmt.Sprintf("* %s%s\n", item.Message, dueToString(item.Due, today))
		}
		str += "\n"
	}
	if str == "" {
		return "Nothing to do in your lists!\n"
	}
	return str
}

// listTitle is a list's icon and name.
func listTitle(list *TodoList) string {
	if list.Icon != "" {
		return list.Icon + " " + list.Name
	}
	return list.Name
}

// assignedListToString lists the todos assigned to someone, for the daily reminder.
func assignedListToString(todos []*AssignedTodo) string {
	if len(todos) == 0 {
		return ""
	}
	str := "\n\nAssigned to you in channels:\n\n"
	for _, todo := range todos {
		where := todo.ListName
		if todo.ChannelName != "" {
			where += ", ~" + todo.ChannelURLName
		}
		str += fmt.Sprintf("* %s\n  * (%s)\n", todo.Message, where)
	}
	return str
}
