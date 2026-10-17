package main

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	MyFlag  = "my"
	InFlag  = "in"
	OutFlag = "out"
)

func getHelp() string {
	return `Available Commands:

lists
	Lists the todo lists of this channel, and your own.

new [name]
	Makes a todo list in this channel, shared with its members.

	example: /todo new Launch checklist

new --personal [name]
	Makes a todo list only you see.

add [message]
	Adds a todo to your list "My todos" (or to the personal list you last added to from this
	channel). Only you see it.

	example: /todo add Don't forget to be awesome

add [list]: [message]
	Adds a todo to a list of this channel or of yours, by its name.

	example: /todo add Launch checklist: Book the room

list
	Lists the todos of your list "My todos".

list [listName]
	Lists the todos sent to you (in) or by you (out).

	example: /todo list in
	example: /todo list out
	example (same as /todo list): /todo list my

pop
	Removes the first todo of your list "My todos".

send [user] [message]
	Sends some user a Todo

	example: /todo send @awesomePerson Don't forget to be awesome

settings summary [on, off]
	Sets user preference on daily reminders

	example: /todo settings summary on

settings allow_incoming_task_requests [on, off]
	Allow other people to send a task for you to accept/decline?

	example: /todo settings allow_incoming_task_requests on


help
	Display usage.
`
}

func getSummarySetting(flag bool) string {
	if flag {
		return "Reminder setting is set to `on`. **You will receive daily reminders.**"
	}
	return "Reminder setting is set to `off`. **You will not receive daily reminders.**"
}
func getAllowIncomingTaskRequestsSetting(flag bool) string {
	if flag {
		return "Allow incoming task requests setting is set to `on`. **Other users can send you task request that you can accept/decline.**"
	}
	return "Allow incoming task requests setting is set to `off`. **Other users cannot send you task request. They will see a message saying you don't accept Todo requests.**"
}

func getAllSettings(summaryFlag, blockIncomingFlag bool) string {
	return fmt.Sprintf(`Current Settings:

%s
%s
	`, getSummarySetting(summaryFlag), getAllowIncomingTaskRequestsSetting(blockIncomingFlag))
}

func getCommand() *model.Command {
	return &model.Command{
		Trigger:          "todo",
		DisplayName:      "Todo Bot",
		Description:      "Interact with your Todo list.",
		AutoComplete:     true,
		AutoCompleteDesc: "Available commands: lists, new, add, list, pop, send, settings, help",
		AutoCompleteHint: "[command]",
		AutocompleteData: getAutocompleteData(),
	}
}

func (p *Plugin) postCommandResponse(args *model.CommandArgs, text string) {
	post := &model.Post{
		UserId:    p.BotUserID,
		ChannelId: args.ChannelId,
		Message:   text,
	}
	_ = p.API.SendEphemeralPost(args.UserId, post)
}

// ExecuteCommand executes a given command and returns a command response.
func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	spaceRegExp := regexp.MustCompile(`\s+`)
	trimmedArgs := spaceRegExp.ReplaceAllString(strings.TrimSpace(args.Command), " ")
	stringArgs := strings.Split(trimmedArgs, " ")
	lengthOfArgs := len(stringArgs)
	restOfArgs := []string{}

	var handler func([]string, *model.CommandArgs) (bool, error)
	if lengthOfArgs == 1 {
		handler = p.runListCommand
	} else {
		command := stringArgs[1]
		if lengthOfArgs > 2 {
			restOfArgs = stringArgs[2:]
		}
		switch command {
		case "lists":
			handler = p.runListsCommand
		case "new":
			handler = p.runNewCommand
		case "add":
			handler = p.runAddCommand
		case "list":
			handler = p.runListCommand
		case "pop":
			handler = p.runPopCommand
		case "send":
			handler = p.runSendCommand
		case "settings":
			handler = p.runSettingsCommand
		default:
			p.postCommandResponse(args, getHelp())
			return &model.CommandResponse{}, nil
		}
	}
	isUserError, err := handler(restOfArgs, args)
	if err != nil {
		if isUserError {
			p.postCommandResponse(args, fmt.Sprintf("__Error: %s.__\n\nRun `/todo help` for usage instructions.", err.Error()))
		} else {
			p.API.LogError(err.Error())
			p.postCommandResponse(args, "An unknown error occurred. Please talk to your system administrator for help.")
		}
	}

	return &model.CommandResponse{}, nil
}

func (p *Plugin) runSendCommand(args []string, extra *model.CommandArgs) (bool, error) {
	if len(args) < 2 {
		p.postCommandResponse(extra, "You must specify a user and a message.\n"+getHelp())
		return false, nil
	}

	userName := args[0]
	if args[0][0] == '@' {
		userName = args[0][1:]
	}
	receiver, appErr := p.API.GetUserByUsername(userName)
	if appErr != nil || p.canSendTo(extra.UserId, receiver.Id) != nil {
		p.postCommandResponse(extra, "Please, provide a valid user.\n"+getHelp())
		return false, nil
	}

	if receiver.Id == extra.UserId {
		return p.runAddCommand(args[1:], extra)
	}

	receiverAllowIncomingTaskRequestsPreference, err := p.getAllowIncomingTaskRequestsPreference(receiver.Id)
	if err != nil {
		p.API.LogError("Error when getting allow incoming task request preference, err=", err)
		receiverAllowIncomingTaskRequestsPreference = true
	}
	if !receiverAllowIncomingTaskRequestsPreference {
		p.postCommandResponse(extra, fmt.Sprintf("@%s has blocked Todo requests", userName))
		return false, nil
	}

	message := strings.Join(args[1:], " ")

	receiverIssueID, err := p.listManager.SendIssue(extra.UserId, receiver.Id, message, "", "", "")
	if err != nil {
		return false, err
	}

	p.sendRefreshEvent(extra.UserId, []string{OutListKey})
	p.sendRefreshEvent(receiver.Id, []string{InListKey})

	responseMessage := fmt.Sprintf("Todo sent to @%s.", userName)

	senderName := p.listManager.GetUserName(extra.UserId)

	receiverMessage := fmt.Sprintf("You have received a new Todo from @%s", senderName)

	postPermalink := ""
	p.PostBotCustomDM(receiver.Id, receiverMessage, message, postPermalink, receiverIssueID)
	p.postCommandResponse(extra, responseMessage)
	return false, nil
}

// where tells whom a list is shared with, for the bot's answers.
func (p *Plugin) where(list *TodoList) string {
	if list.ChannelID == "" {
		return "only you can see it"
	}
	channel, appErr := p.API.GetChannel(list.ChannelID)
	if appErr != nil || isConversation(channel) {
		return "shared with this conversation"
	}
	return "shared with ~" + channel.Name
}

func today() string { return time.Now().Format(time.DateOnly) }

// runListsCommand lists the lists of the channel and the person's own.
func (p *Plugin) runListsCommand(_ []string, extra *model.CommandArgs) (bool, error) {
	var b strings.Builder
	if channel, err := p.channelRights(extra.UserId, extra.ChannelId); err == nil && channel.Member {
		lists, err := p.ChannelLists(extra.UserId, extra.ChannelId)
		if err != nil {
			return false, err
		}
		if isConversation(channel.Channel) {
			b.WriteString("#### Todo lists of this conversation\n")
		} else {
			fmt.Fprintf(&b, "#### Todo lists of ~%s\n", channel.Channel.Name)
		}
		writeLists(&b, lists, "No lists here yet: `/todo new <name>` makes one.")
	}
	lists, err := p.PersonalLists(extra.UserId)
	if err != nil {
		return false, err
	}
	b.WriteString("\n#### Your lists\n")
	writeLists(&b, lists, "")
	p.postCommandResponse(extra, b.String())
	return false, nil
}

func writeLists(b *strings.Builder, lists []ListInfo, empty string) {
	shown := 0
	for _, list := range lists {
		if list.ArchivedAt != 0 {
			continue
		}
		shown++
		fmt.Fprintf(b, "* **%s** (%d open%s)\n", listTitle(&list.TodoList), list.OpenCount, dueToString(list.EarliestDue, today()))
	}
	if shown == 0 && empty != "" {
		b.WriteString(empty + "\n")
	}
}

// runNewCommand makes a list of the channel, or with --personal, one of the person's own.
func (p *Plugin) runNewCommand(args []string, extra *model.CommandArgs) (bool, error) {
	req := listRequest{ChannelID: extra.ChannelId}
	if len(args) > 0 && (args[0] == "--personal" || args[0] == "--mine") {
		req.ChannelID = ""
		args = args[1:]
	}
	name := strings.Join(args, " ")
	if strings.TrimSpace(name) == "" {
		return true, errors.New("give the list a name: `/todo new <name>`")
	}
	req.Name = &name
	list, err := p.CreateList(extra.UserId, req)
	if err != nil {
		if status := errorStatus(err); status == http.StatusBadRequest || status == http.StatusForbidden {
			return true, err
		}
		return false, err
	}
	p.postCommandResponse(extra, fmt.Sprintf("Made the list **%s**, %s.", listTitle(&list.TodoList), p.where(&list.TodoList)))
	return false, nil
}

// findList finds a list someone can add to by its name: one of the channel's first, then theirs.
func (p *Plugin) findList(userID, channelID, name string) *TodoList {
	targets, err := p.Targets(userID, channelID)
	if err != nil {
		return nil
	}
	name = strings.ToLower(strings.Join(strings.Fields(name), " "))
	for _, info := range targets.Lists {
		if strings.ToLower(info.Name) == name || strings.ToLower(listTitle(&info.TodoList)) == name {
			list := info.TodoList
			return &list
		}
	}
	return nil
}

// commandTarget is the list "/todo add" adds to when no list is named: the list last used from the
// channel if it's one of the person's own, else their default list. Never a shared list unasked:
// the command doesn't show where the todo goes before it's added (the message's "Add to todo"
// does, and offers the channel's lists).
func (p *Plugin) commandTarget(userID, channelID string) (*TodoList, error) {
	if last := p.lastUsedList(userID, channelID); last != "" {
		if list, err := p.getList(last); err == nil && list != nil && list.ChannelID == "" {
			if rights, err := p.listRights(userID, list); err == nil && rights.Edit {
				return list, nil
			}
		}
	}
	return p.ensurePersonal(userID)
}

// runAddCommand adds a todo: to the list named before a colon, or else to a personal list.
func (p *Plugin) runAddCommand(args []string, extra *model.CommandArgs) (bool, error) {
	message := strings.Join(args, " ")

	if message == "" {
		p.postCommandResponse(extra, "Please add a task.")
		return false, nil
	}

	var list *TodoList
	if name, rest, found := strings.Cut(message, ":"); found && strings.TrimSpace(rest) != "" {
		if list = p.findList(extra.UserId, extra.ChannelId, name); list != nil {
			message = strings.TrimSpace(rest)
		}
	}
	if list == nil {
		var err error
		if list, err = p.commandTarget(extra.UserId, extra.ChannelId); err != nil {
			return false, err
		}
	}

	if _, err := p.AddItem(extra.UserId, list.ID, itemRequest{Message: &message, ChannelID: extra.ChannelId}); err != nil {
		if errorStatus(err) == http.StatusBadRequest {
			return true, err
		}
		return false, err
	}

	responseMessage := fmt.Sprintf("Added to **%s** (%s).", listTitle(list), p.where(list))
	if items, err := p.getItems(list.ID); err == nil {
		open := []*Item{}
		for _, item := range items {
			if !item.Done() {
				open = append(open, item)
			}
		}
		responseMessage += "\n\n" + itemsToString(open, today())
	}
	p.postCommandResponse(extra, responseMessage)
	return false, nil
}

func (p *Plugin) runListCommand(args []string, extra *model.CommandArgs) (bool, error) {
	listID := MyListKey
	responseMessage := "Todo List:\n\n"

	if len(args) > 0 {
		switch args[0] {
		case MyFlag:
		case InFlag:
			listID = InListKey
			responseMessage = "Received Todo list:\n\n"
		case OutFlag:
			listID = OutListKey
			responseMessage = "Sent Todo list:\n\n"
		default:
			p.postCommandResponse(extra, getHelp())
			return true, nil
		}
	}

	if listID == MyListKey {
		list, err := p.ensurePersonal(extra.UserId)
		if err != nil {
			return false, err
		}
		items, err := p.getItems(list.ID)
		if err != nil {
			return false, err
		}
		open := []*Item{}
		for _, item := range items {
			if !item.Done() {
				open = append(open, item)
			}
		}
		p.postCommandResponse(extra, fmt.Sprintf("**%s**:\n\n%s\nYour other lists: `/todo lists`.", listTitle(list), itemsToString(open, today())))
		return false, nil
	}

	issues, err := p.listManager.GetIssueList(extra.UserId, listID)
	if err != nil {
		return false, err
	}

	p.sendRefreshEvent(extra.UserId, []string{MyListKey, OutListKey, InListKey})

	responseMessage += issuesListToString(issues)
	p.postCommandResponse(extra, responseMessage)

	return false, nil
}

// runPopCommand removes the first open todo of the person's default list.
func (p *Plugin) runPopCommand(_ []string, extra *model.CommandArgs) (bool, error) {
	list, err := p.ensurePersonal(extra.UserId)
	if err != nil {
		return false, err
	}
	items, err := p.getItems(list.ID)
	if err != nil {
		return false, err
	}
	i := slices.IndexFunc(items, func(t *Item) bool { return !t.Done() })
	if i < 0 {
		p.postCommandResponse(extra, "There are no Todos to pop.")
		return false, nil
	}
	popped, err := p.takeItem(list.ID, items[i].ID)
	if err != nil {
		return false, err
	}
	p.sendListChanged(list)

	userName := p.listManager.GetUserName(extra.UserId)
	if popped.FromIssueID != "" {
		if err = p.listManager.ReleaseRequest(popped.ID, popped.FromUserID, popped.FromIssueID); err != nil {
			p.API.LogWarn("Could not remove the sender's copy of a todo", "error", err.Error())
		}
		p.sendRefreshEvent(popped.FromUserID, []string{OutListKey})
		p.PostBotDM(popped.FromUserID, fmt.Sprintf("@%s popped a Todo you sent: %s", userName, popped.Message))
	}
	if popped.PostID != "" {
		p.postReplyIfNeeded(popped.PostID, fmt.Sprintf("@%s popped a todo attached to this thread", userName), popped.Message, popped.PostPermalink)
	}

	open := []*Item{}
	for _, item := range items {
		if !item.Done() && item.ID != popped.ID {
			open = append(open, item)
		}
	}
	p.postCommandResponse(extra, fmt.Sprintf("Removed the first todo of **%s**.\n\n%s", listTitle(list), itemsToString(open, today())))
	return false, nil
}

func (p *Plugin) runSettingsCommand(args []string, extra *model.CommandArgs) (bool, error) {
	const (
		on  = "on"
		off = "off"
	)
	if len(args) < 1 {
		currentSummarySetting := p.getReminderPreference(extra.UserId)
		currentAllowIncomingTaskRequestsSetting, err := p.getAllowIncomingTaskRequestsPreference(extra.UserId)
		if err != nil {
			p.API.LogError("Error when getting allow incoming task request preference, err=", err)
			currentAllowIncomingTaskRequestsSetting = true
		}
		p.postCommandResponse(extra, getAllSettings(currentSummarySetting, currentAllowIncomingTaskRequestsSetting))
		return false, nil
	}

	switch args[0] {
	case "summary":
		if len(args) < 2 {
			currentSummarySetting := p.getReminderPreference(extra.UserId)
			p.postCommandResponse(extra, getSummarySetting(currentSummarySetting))
			return false, nil
		}
		if len(args) > 2 {
			return true, errors.New("too many arguments")
		}
		var responseMessage string
		var err error

		switch args[1] {
		case on:
			err = p.saveReminderPreference(extra.UserId, true)
			responseMessage = "You will start receiving daily summaries."
		case off:
			err = p.saveReminderPreference(extra.UserId, false)
			responseMessage = "You will stop receiving daily summaries."
		default:
			responseMessage = "invalid input, allowed values for \"settings summary\" are `on` or `off`"
			return true, errors.New(responseMessage)
		}

		if err != nil {
			responseMessage = "error saving the reminder preference"
			p.API.LogDebug("runSettingsCommand: error saving the reminder preference", "error", err.Error())
			return false, errors.New(responseMessage)
		}

		p.postCommandResponse(extra, responseMessage)

	case "allow_incoming_task_requests":
		if len(args) < 2 {
			currentAllowIncomingTaskRequestsSetting, err := p.getAllowIncomingTaskRequestsPreference(extra.UserId)
			if err != nil {
				p.API.LogError("unable to parse the allow incoming task requests preference, err=", err.Error())
				currentAllowIncomingTaskRequestsSetting = true
			}
			p.postCommandResponse(extra, getAllowIncomingTaskRequestsSetting(currentAllowIncomingTaskRequestsSetting))
			return false, nil
		}
		if len(args) > 2 {
			return true, errors.New("too many arguments")
		}
		var responseMessage string
		var err error

		switch args[1] {
		case on:
			err = p.saveAllowIncomingTaskRequestsPreference(extra.UserId, true)
			responseMessage = "Other users can send task for you to accept/decline"
		case off:
			err = p.saveAllowIncomingTaskRequestsPreference(extra.UserId, false)
			responseMessage = "Other users cannot send you task request. They will see a message saying you have blocked incoming task requests"
		default:
			responseMessage = "invalid input, allowed values for \"settings allow_incoming_task_requests\" are `on` or `off`"
			return true, errors.New(responseMessage)
		}

		if err != nil {
			responseMessage = "error saving the block_incoming preference"
			p.API.LogDebug("runSettingsCommand: error saving the block_incoming preference", "error", err.Error())
			return false, errors.New(responseMessage)
		}

		p.postCommandResponse(extra, responseMessage)
	default:
		return true, fmt.Errorf("setting `%s` not recognized", args[0])
	}
	return false, nil
}

func getAutocompleteData() *model.AutocompleteData {
	todo := model.NewAutocompleteData("todo", "[command]", "Available commands: lists, new, add, list, pop, send, settings, help")

	lists := model.NewAutocompleteData("lists", "", "Lists the todo lists of this channel, and yours")
	todo.AddCommand(lists)

	newList := model.NewAutocompleteData("new", "[--personal] [name]", "Makes a todo list in this channel (--personal: only you see it)")
	newList.AddTextArgument("The list's name", "[--personal] [name]", "")
	todo.AddCommand(newList)

	add := model.NewAutocompleteData("add", "[list:] [message]", "Adds a Todo, to the list named before a colon")
	add.AddTextArgument("E.g. Launch: be awesome", "[list:] [message]", "")
	todo.AddCommand(add)

	list := model.NewAutocompleteData("list", "[name]", "Lists your Todo issues")
	items := []model.AutocompleteListItem{{
		HelpText: "Received Todos",
		Hint:     "(optional)",
		Item:     "in",
	}, {
		HelpText: "Sent Todos",
		Hint:     "(optional)",
		Item:     "out",
	}}
	list.AddStaticListArgument("Lists your Todo issues", false, items)
	todo.AddCommand(list)

	pop := model.NewAutocompleteData("pop", "", "Removes the first todo of your list \"My todos\"")
	todo.AddCommand(pop)

	send := model.NewAutocompleteData("send", "[user] [todo]", "Sends a Todo to a specified user")
	send.AddTextArgument("Whom to send", "[@awesomePerson]", "")
	send.AddTextArgument("Todo message", "[message]", "")
	todo.AddCommand(send)

	settings := model.NewAutocompleteData("settings", "[setting] [on] [off]", "Sets the user settings")
	summary := model.NewAutocompleteData("summary", "[on] [off]", "Sets the summary settings")
	summaryOn := model.NewAutocompleteData("on", "", "sets the daily reminder to enable")
	summaryOff := model.NewAutocompleteData("off", "", "sets the daily reminder to disable")
	summary.AddCommand(summaryOn)
	summary.AddCommand(summaryOff)

	allowIncomingTask := model.NewAutocompleteData("allow_incoming_task_requests", "[on] [off]", "Allow other people to send a task for you to accept/decline?")
	allowIncomingTaskOn := model.NewAutocompleteData("on", "", "Allow others to send you a Task, you can accept/decline")
	allowIncomingTaskOff := model.NewAutocompleteData("off", "", "Block others from sending you a Task, they will see a message saying you don't accept Todo requests")
	allowIncomingTask.AddCommand(allowIncomingTaskOn)
	allowIncomingTask.AddCommand(allowIncomingTaskOff)

	settings.AddCommand(summary)
	settings.AddCommand(allowIncomingTask)
	todo.AddCommand(settings)

	help := model.NewAutocompleteData("help", "", "Display usage")
	todo.AddCommand(help)
	return todo
}
