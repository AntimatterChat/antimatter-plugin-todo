package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
)

const (
	// WSEventRefresh is the WebSocket event for refreshing the Todo list
	WSEventRefresh = "refresh"

	ErrorMsgAddIssue = "Unable to add issue"
)

// ListManager represents the logic of the todos people send each other ("requests"); the lists
// themselves are in lists.go.
type ListManager interface {
	// SendIssue sends the todo with the message from senderID to receiverID and returns the receiver's issueID
	SendIssue(senderID, receiverID, message, postPermalink, description, postID string) (string, error)
	// GetIssueList gets the todos on listID for userID
	GetIssueList(userID, listID string) ([]*ExtendedIssue, error)
	// GetAllList get all issues
	GetAllList(userID string) (*ListsIssue, error)
	// CompleteIssue completes the todo issueID for userID, and returns the issue and the foreign ID if any
	CompleteIssue(userID, issueID string) (issue *Issue, foreignID string, listToUpdate string, err error)
	// GetRequest returns a todo sent to userID, waiting for their answer, and its reference
	GetRequest(userID, issueID string) (*Issue, *IssueRef, error)
	// LinkRequest records that a todo sent to userID is in their list listID now, and no longer waiting
	LinkRequest(userID, issueID, listID string) error
	// ReleaseRequest removes the receiver's copy issueID of an accepted todo, and the sender's
	ReleaseRequest(issueID, senderID, senderIssueID string) error
	// UpdateRequestText changes the text of both copies of an accepted todo, and returns the old one
	UpdateRequestText(senderIssueID, issueID, message, description string) (oldMessage string, err error)
	// RemoveIssue removes the todo issueID for userID and returns the issue, the foreign ID if any and whether the user sent the todo to someone else
	RemoveIssue(userID, issueID string) (issue *Issue, foreignID string, isSender bool, listToUpdate string, err error)
	// BumpIssue moves a issueID sent by userID to the top of its receiver inbox list
	BumpIssue(userID string, issueID string) (todo *Issue, receiver string, foreignIssueID string, err error)
	// EditIssue updates the message on an issue
	EditIssue(userID string, issueID string, newMessage string, newDescription string) (foreignUserID string, list string, oldMessage string, err error)
	// ChangeAssignment updates an issue to assign a different person
	ChangeAssignment(issueID string, userID string, sendTo string) (issue *Issue, oldOwner string, err error)
	// GetUserName returns the readable username from userID
	GetUserName(userID string) string
}

// Plugin implements the interface expected by the Antimatter server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin
	client *pluginapi.Client

	BotUserID string

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	router *mux.Router

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	listManager ListManager
}

func (p *Plugin) OnActivate() error {
	config := p.getConfiguration()
	if err := config.IsValid(); err != nil {
		return err
	}

	if p.client == nil {
		p.client = pluginapi.NewClient(p.API, p.Driver)
	}

	botID, err := p.client.Bot.EnsureBot(&model.Bot{
		Username:    "todo",
		DisplayName: "Todo Bot",
		Description: "Created by the Todo app.",
	})
	if err != nil {
		return errors.Wrap(err, "failed to ensure todo bot")
	}
	p.BotUserID = botID

	p.listManager = NewListManager(p.API, p)

	p.initializeAPI()

	// Todos of before lists are migrated when they're first read; this migrates everyone's.
	go func() {
		if err := p.MigrateAll(); err != nil {
			p.API.LogError("Could not migrate the todos to lists", "error", err.Error())
		}
	}()

	return p.API.RegisterCommand(getCommand())
}

func (p *Plugin) initializeAPI() {
	p.router = mux.NewRouter()
	p.router.Use(p.withRecovery)

	p.router.HandleFunc("/add", p.checkAuth(p.handleAdd)).Methods(http.MethodPost)
	p.router.HandleFunc("/lists", p.checkAuth(p.handleLists)).Methods(http.MethodGet)
	p.router.HandleFunc("/remove", p.checkAuth(p.handleRemove)).Methods(http.MethodPost)
	p.router.HandleFunc("/complete", p.checkAuth(p.handleComplete)).Methods(http.MethodPost)
	p.router.HandleFunc("/accept", p.checkAuth(p.handleAccept)).Methods(http.MethodPost)
	p.router.HandleFunc("/bump", p.checkAuth(p.handleBump)).Methods(http.MethodPost)
	p.router.HandleFunc("/edit", p.checkAuth(p.handleEdit)).Methods(http.MethodPut)
	p.router.HandleFunc("/change_assignment", p.checkAuth(p.handleChangeAssignment)).Methods(http.MethodPost)

	p.initListsAPI(p.router)

	// 404 handler
	p.router.Handle("{anything:.*}", http.NotFoundHandler())
}

// ServeHTTP demonstrates a plugin that handles HTTP requests by greeting the world.
func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	p.router.ServeHTTP(w, r)
}

func (p *Plugin) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if x := recover(); x != nil {
				p.API.LogWarn("Recovered from a panic",
					"url", r.URL.String(),
					"error", x,
					"stack", string(debug.Stack()))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func (p *Plugin) checkAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("Mattermost-User-ID")
		if userID == "" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		handler(w, r)
	}
}

// handleAdd adds a todo to the person's default list, or sends it to someone (the API of before
// lists; the lists' own is in api.go).
func (p *Plugin) handleAdd(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")

	addRequest, err := GetAddIssuePayloadFromJSON(r.Body)
	if err != nil {
		msg := "Unable to get add issue payload from JSON"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusBadRequest, msg, err)
		return
	}

	if err = addRequest.IsValid(); err != nil {
		p.handleErrorWithCode(w, http.StatusBadRequest, "Unable to validate add issue payload.", err)
		return
	}

	// A todo is only linked to a message its author can read: the bot answers in its thread.
	postPermalink := ""
	if addRequest.PostID != "" {
		var post *model.Post
		if post, postPermalink, err = p.readablePost(userID, addRequest.PostID); err != nil {
			p.handleErrorWithCode(w, http.StatusBadRequest, "Unable to find the message", err)
			return
		}
		addRequest.PostID = post.Id
	}

	var receiver *model.User
	if addRequest.SendTo != "" {
		var appErr *model.AppError
		if receiver, appErr = p.API.GetUserByUsername(addRequest.SendTo); appErr != nil {
			msg := "Unable to find user"
			p.API.LogError(msg, "err", appErr.Error())
			p.handleErrorWithCode(w, http.StatusInternalServerError, msg, appErr)
			return
		}
		if err = p.canSendTo(userID, receiver.Id); err != nil {
			p.handleErrorWithCode(w, errorStatus(err), "Unable to find user", err)
			return
		}
	}

	if receiver == nil || receiver.Id == userID {
		list, listErr := p.ensurePersonal(userID)
		if listErr == nil {
			message, description := addRequest.Message, addRequest.Description
			_, listErr = p.AddItem(userID, list.ID, itemRequest{Message: &message, Description: &description, PostID: addRequest.PostID})
		}
		if listErr != nil {
			p.API.LogError(ErrorMsgAddIssue, "err", listErr.Error())
			p.handleErrorWithCode(w, errorStatus(listErr), ErrorMsgAddIssue, listErr)
		}
		return
	}

	receiverAllowIncomingTaskRequestsPreference, err := p.getAllowIncomingTaskRequestsPreference(receiver.Id)
	if err != nil {
		p.API.LogError("Error when getting allow incoming task request preference, err=", err)
		receiverAllowIncomingTaskRequestsPreference = true
	}
	if !receiverAllowIncomingTaskRequestsPreference {
		replyMessage := fmt.Sprintf("@%s has blocked Todo requests", receiver.Username)
		p.PostBotDM(userID, replyMessage)
		return
	}

	senderName := p.listManager.GetUserName(userID)
	issueID, err := p.listManager.SendIssue(userID, receiver.Id, addRequest.Message, postPermalink, addRequest.Description, addRequest.PostID)
	if err != nil {
		msg := "Unable to send issue"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusInternalServerError, msg, err)
		return
	}

	p.sendRefreshEvent(userID, []string{OutListKey})
	p.sendRefreshEvent(receiver.Id, []string{InListKey})

	receiverMessage := fmt.Sprintf("You have received a new Todo from @%s", senderName)
	p.PostBotCustomDM(receiver.Id, receiverMessage, addRequest.Message, postPermalink, issueID)

	replyMessage := fmt.Sprintf("@%s sent @%s a todo attached to this thread", senderName, receiver.Username)
	p.postReplyIfNeeded(addRequest.PostID, replyMessage, addRequest.Message, postPermalink)
}

func (p *Plugin) postReplyIfNeeded(postID, message, todo, postPermalink string) {
	if postID != "" {
		err := p.ReplyPostBot(postID, message, todo, postPermalink)
		if err != nil {
			p.API.LogError(err.Error())
		}
	}
}

// personalOpenItems returns the open todos of someone's personal lists that aren't archived.
func (p *Plugin) personalOpenItems(userID string) ([]*TodoList, map[string][]*Item, error) {
	if _, err := p.ensurePersonal(userID); err != nil {
		return nil, nil, err
	}
	lists, err := p.listsOf(userListsKey(userID))
	if err != nil {
		return nil, nil, err
	}
	open := map[string][]*Item{}
	kept := lists[:0]
	for _, list := range lists {
		if list.ArchivedAt != 0 || list.OwnerID != userID {
			continue
		}
		items, err := p.getItems(list.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range items {
			if !item.Done() {
				open[list.ID] = append(open[list.ID], item)
			}
		}
		kept = append(kept, list)
	}
	return kept, open, nil
}

// handleLists returns the todos someone sent and received (and, as "my", the open todos of their
// personal lists). With reminder=true, it also sends the daily reminder when it's due.
func (p *Plugin) handleLists(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")

	allListIssue, err := p.listManager.GetAllList(userID)
	if err != nil {
		msg := "Unable to get issues for user"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusInternalServerError, msg, err)
		return
	}
	lists, open, err := p.personalOpenItems(userID)
	if err != nil {
		msg := "Unable to get the lists of the user"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusInternalServerError, msg, err)
		return
	}
	allListIssue.My = []*ExtendedIssue{}
	for _, list := range lists {
		for _, item := range open[list.ID] {
			allListIssue.My = append(allListIssue.My, &ExtendedIssue{Issue: Issue{ID: item.ID, Message: item.Message, Description: item.Description, PostPermalink: item.PostPermalink, PostID: item.PostID, CreateAt: item.CreateAt, ListID: list.ID}})
		}
	}

	reminder := r.URL.Query().Get("reminder") == "true" && p.getReminderPreference(userID)
	var assigned []*AssignedTodo
	if reminder {
		if assigned, err = p.AssignedTodos(userID); err != nil {
			p.API.LogWarn("Unable to get the assigned todos for the reminder", "err", err.Error())
		}
	}
	if allListIssue != nil && (len(allListIssue.My) > 0 || len(assigned) > 0) && reminder {
		var lastReminderAt int64
		lastReminderAt, err = p.getLastReminderTimeForUser(userID)
		if err != nil {
			msg := "Unable to send reminder"
			p.API.LogError(msg, "err", err.Error())
			p.handleErrorWithCode(w, http.StatusInternalServerError, msg, err)
			return
		}

		var timezone *time.Location
		offset, _ := strconv.Atoi(r.Header.Get("X-Timezone-Offset"))
		timezone = time.FixedZone("local", -60*offset)

		// Post reminder message if it's the next day and been more than an hour since the last post
		now := model.GetMillis()
		nt := time.Unix(now/1000, 0).In(timezone)
		lt := time.Unix(lastReminderAt/1000, 0).In(timezone)
		if nt.Sub(lt).Hours() >= 1 && (nt.Day() != lt.Day() || nt.Month() != lt.Month() || nt.Year() != lt.Year()) {
			p.PostBotDM(userID, "Daily Reminder:\n\n"+personalListsToString(lists, open, nt.Format(time.DateOnly))+assignedListToString(assigned))
			err = p.saveLastReminderTimeForUser(userID)
			if err != nil {
				p.API.LogError("Unable to save last reminder for user err=" + err.Error())
			}
		}
	}

	allListIssueJSON, err := json.Marshal(allListIssue)
	if err != nil {
		msg := "Unable marhsal all lists issues to json"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusInternalServerError, msg, err)
		return
	}

	_, err = w.Write(allListIssueJSON)
	if err != nil {
		p.API.LogError("Unable to write json response while listing issues err=" + err.Error())
	}
}

func (p *Plugin) handleEdit(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")

	editRequest, err := GetEditIssuePayloadFromJSON(r.Body)
	if err != nil {
		msg := "Unable to get edit issue payload from JSON"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusBadRequest, msg, err)
		return
	}

	if err = editRequest.IsValid(); err != nil {
		p.handleErrorWithCode(w, http.StatusBadRequest, "Unable to validate edit issue payload.", err)
		return
	}

	foreignUserID, list, oldMessage, err := p.listManager.EditIssue(userID, editRequest.ID, editRequest.Message, editRequest.Description)
	if err != nil {
		msg := "Unable to edit message"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusInternalServerError, msg, err)
		return
	}

	p.sendRefreshEvent(userID, []string{list})

	if foreignUserID != "" {
		var lists []string
		if list == OutListKey {
			lists = []string{MyListKey, InListKey}
		} else {
			lists = []string{OutListKey}
		}
		p.sendRefreshEvent(foreignUserID, lists)

		userName := p.listManager.GetUserName(userID)
		message := fmt.Sprintf("@%s modified a Todo from:\n%s\nTo:\n%s", userName, oldMessage, editRequest.Message)
		p.PostBotDM(foreignUserID, message)
	}
}

func (p *Plugin) handleChangeAssignment(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")

	changeRequest, err := GetChangeAssignmentPayloadFromJSON(r.Body)
	if err != nil {
		msg := "Unable to get change request payload from JSON"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusBadRequest, msg, err)
		return
	}

	if err = changeRequest.IsValid(); err != nil {
		p.handleErrorWithCode(w, http.StatusBadRequest, "Unable to validate change request payload.", err)
		return
	}

	receiver, appErr := p.API.GetUserByUsername(changeRequest.SendTo)
	if appErr != nil {
		msg := "username not valid"
		p.API.LogError(msg, "err", appErr.Error())
		p.handleErrorWithCode(w, http.StatusNotFound, msg, appErr)
		return
	}
	if err = p.canSendTo(userID, receiver.Id); err != nil {
		p.handleErrorWithCode(w, http.StatusNotFound, "username not valid", err)
		return
	}

	issue, oldOwner, err := p.listManager.ChangeAssignment(changeRequest.ID, userID, receiver.Id)
	if err != nil {
		msg := "Unable to change the assignment of an issue"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusInternalServerError, msg, err)
		return
	}

	p.sendRefreshEvent(userID, []string{MyListKey, OutListKey})

	userName := p.listManager.GetUserName(userID)
	if receiver.Id != userID {
		p.sendRefreshEvent(receiver.Id, []string{InListKey})
		receiverMessage := fmt.Sprintf("You have received a new Todo from @%s", userName)
		p.PostBotCustomDM(receiver.Id, receiverMessage, issue.Message, issue.PostPermalink, changeRequest.ID)
	}
	if oldOwner != "" {
		p.sendRefreshEvent(oldOwner, []string{InListKey, MyListKey})
		oldOwnerMessage := fmt.Sprintf("@%s removed you from Todo:\n%s", userName, issue.Message)
		p.PostBotDM(oldOwner, oldOwnerMessage)
	}
}

func (p *Plugin) handleAccept(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")

	acceptRequest, err := GetAcceptRequestPayloadFromJSON(r.Body)
	if err != nil {
		msg := "Unable to get accept request payload from JSON"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusBadRequest, msg, err)
		return
	}

	if err = acceptRequest.IsValid(); err != nil {
		p.handleErrorWithCode(w, http.StatusBadRequest, "Unable to validate accept request payload.", err)
		return
	}

	todoMessage, sender, err := p.acceptRequest(userID, acceptRequest.ID, acceptRequest.ListID)
	if err != nil {
		msg := "Unable to accept issue"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, errorStatus(err), msg, err)
		return
	}

	p.sendRefreshEvent(userID, []string{MyListKey, InListKey})
	p.sendRefreshEvent(sender, []string{OutListKey})

	userName := p.listManager.GetUserName(userID)
	message := fmt.Sprintf("@%s accepted a Todo you sent: %s", userName, todoMessage)
	p.PostBotDM(sender, message)
}

func (p *Plugin) handleComplete(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")

	completeRequest, err := GetCompleteIssuePayloadFromJSON(r.Body)
	if err != nil {
		msg := "Unable to get complete issue request payload from JSON"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusBadRequest, msg, err)
		return
	}

	if err = completeRequest.IsValid(); err != nil {
		p.handleErrorWithCode(w, http.StatusBadRequest, "Unable to validate complete issue request payload.", err)
		return
	}

	issue, foreignID, listToUpdate, err := p.listManager.CompleteIssue(userID, completeRequest.ID)
	if err != nil {
		msg := "Unable to complete issue"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusInternalServerError, msg, err)
		return
	}

	p.sendRefreshEvent(userID, []string{listToUpdate})

	userName := p.listManager.GetUserName(userID)
	replyMessage := fmt.Sprintf("@%s completed a todo attached to this thread", userName)
	p.postReplyIfNeeded(issue.PostID, replyMessage, issue.Message, issue.PostPermalink)

	if foreignID == "" {
		return
	}

	p.sendRefreshEvent(foreignID, []string{OutListKey})

	if issue.PostPermalink != "" {
		issue.Message = fmt.Sprintf("%s\n[Permalink](%s)", issue.Message, issue.PostPermalink)
	}

	message := fmt.Sprintf("@%s completed a Todo you sent: %s", userName, issue.Message)
	p.PostBotDM(foreignID, message)
}

func (p *Plugin) handleRemove(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")

	removeRequest, err := GetRemoveIssuePayloadFromJSON(r.Body)
	if err != nil {
		msg := "Unable to get remove issue request payload from JSON"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusBadRequest, msg, err)
		return
	}

	if err = removeRequest.IsValid(); err != nil {
		p.handleErrorWithCode(w, http.StatusBadRequest, "Unable to validate remove issue request payload.", err)
		return
	}

	issue, foreignID, isSender, listToUpdate, err := p.listManager.RemoveIssue(userID, removeRequest.ID)
	if err != nil {
		msg := "Unable to remove issue"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusInternalServerError, msg, err)
		return
	}
	p.sendRefreshEvent(userID, []string{listToUpdate})

	userName := p.listManager.GetUserName(userID)
	replyMessage := fmt.Sprintf("@%s removed a todo attached to this thread", userName)
	p.postReplyIfNeeded(issue.PostID, replyMessage, issue.Message, issue.PostPermalink)

	if foreignID == "" {
		return
	}

	list := InListKey

	message := fmt.Sprintf("@%s removed a Todo you received: %s", userName, issue.Message)
	if isSender {
		message = fmt.Sprintf("@%s declined a Todo you sent: %s", userName, issue.Message)
		list = OutListKey
	}

	if issue.PostPermalink != "" {
		message = fmt.Sprintf("%s\n[Permalink](%s)", message, issue.PostPermalink)
	}

	p.sendRefreshEvent(foreignID, []string{list})

	p.PostBotDM(foreignID, message)
}

func (p *Plugin) handleBump(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")

	bumpRequest, err := GetBumpIssuePayloadFromJSON(r.Body)
	if err != nil {
		msg := "Unable to get bump issue request payload from JSON"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusBadRequest, msg, err)
		return
	}

	if err = bumpRequest.IsValid(); err != nil {
		p.handleErrorWithCode(w, http.StatusBadRequest, "Unable to validate bump request payload.", err)
		return
	}

	todo, foreignUser, foreignIssueID, err := p.listManager.BumpIssue(userID, bumpRequest.ID)
	if err != nil {
		msg := "Unable to bump issue"
		p.API.LogError(msg, "err", err.Error())
		p.handleErrorWithCode(w, http.StatusInternalServerError, msg, err)
		return
	}

	if foreignUser == "" {
		return
	}

	p.sendRefreshEvent(foreignUser, []string{InListKey})

	userName := p.listManager.GetUserName(userID)
	message := fmt.Sprintf("@%s bumped a Todo you received.", userName)
	p.PostBotCustomDM(foreignUser, message, todo.Message, todo.PostPermalink, foreignIssueID)
}

func (p *Plugin) sendRefreshEvent(userID string, lists []string) {
	p.API.PublishWebSocketEvent(
		WSEventRefresh,
		map[string]interface{}{"lists": lists},
		&model.WebsocketBroadcast{UserId: userID},
	)
}

func (p *Plugin) handleErrorWithCode(w http.ResponseWriter, code int, errTitle string, err error) {
	w.WriteHeader(code)
	b, _ := json.Marshal(struct {
		Error   string `json:"error"`
		Details string `json:"details"`
	}{
		Error:   errTitle,
		Details: err.Error(),
	})
	_, _ = w.Write(b)
}
