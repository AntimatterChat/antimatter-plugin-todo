package main

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/pkg/errors"
)

const (
	// MyListKey is the key used to store the list of the owned todos
	MyListKey = ""
	// InListKey is the key used to store the list of received todos
	InListKey = "_in"
	// OutListKey is the key used to store the list of sent todos
	OutListKey = "_out"
)

// ListStore represents the KVStore operations for lists
type ListStore interface {
	// Issue related function
	SaveIssue(issue *Issue) error
	GetIssue(issueID string) (*Issue, error)
	RemoveIssue(issueID string) error
	GetAndRemoveIssue(issueID string) (*Issue, error)

	// Issue References related functions

	// AddReference creates a new IssueRef with the issueID, foreignUSerID and foreignIssueID, and stores it
	// on the listID for userID.
	AddReference(userID, issueID, listID, foreignUserID, foreignIssueID string) error
	// RemoveReference removes the IssueRef for issueID in listID for userID
	RemoveReference(userID, issueID, listID string) error
	// PopReference removes the first IssueRef in listID for userID and returns it
	PopReference(userID, listID string) (*IssueRef, error)
	// BumpReference moves the Issue reference for issueID in listID for userID to the beginning of the list
	BumpReference(userID, issueID, listID string) error
	// GetIssueReference gets the IssueRef and position of the issue issueID on user userID's list listID
	GetIssueReference(userID, issueID, listID string) (*IssueRef, int, error)
	// GetIssueListAndReference gets the issue list, IssueRef and position for user userID
	GetIssueListAndReference(userID, issueID string) (string, *IssueRef, int)
	// GetList returns the list of IssueRef in listID for userID
	GetList(userID, listID string) ([]*IssueRef, error)
}

// LinkedItems reaches the todos people accepted into their lists, from their senders' side.
type LinkedItems interface {
	// RemoveLinkedItem takes an accepted todo out of its list: its sender cancelled it.
	RemoveLinkedItem(listID, itemID string)
	// EditLinkedItem changes the text of an accepted todo: its sender edited it.
	EditLinkedItem(listID, itemID, message, description string)
}

type listManager struct {
	store  ListStore
	api    plugin.API
	linked LinkedItems
}

// NewListManager creates a new listManager; linked reaches the todos accepted into lists.
func NewListManager(api plugin.API, linked LinkedItems) ListManager {
	return &listManager{
		store:  NewListStore(api),
		api:    api,
		linked: linked,
	}
}

// removeLinked takes an accepted todo out of its receiver's list, if it's in one.
func (l *listManager) removeLinked(issue *Issue) {
	if issue != nil && issue.ListID != "" && l.linked != nil {
		l.linked.RemoveLinkedItem(issue.ListID, issue.ID)
	}
}

func (l *listManager) SendIssue(senderID, receiverID, message, postPermalink, description, postID string) (string, error) {
	senderIssue := newIssue(message, postPermalink, description, postID)
	if err := l.store.SaveIssue(senderIssue); err != nil {
		return "", err
	}

	receiverIssue := newIssue(message, postPermalink, description, postID)
	if err := l.store.SaveIssue(receiverIssue); err != nil {
		if rollbackError := l.store.RemoveIssue(senderIssue.ID); rollbackError != nil {
			l.api.LogError("cannot rollback sender issue after send error, Err=", err.Error())
		}
		return "", err
	}

	if err := l.store.AddReference(senderID, senderIssue.ID, OutListKey, receiverID, receiverIssue.ID); err != nil {
		if rollbackError := l.store.RemoveIssue(senderIssue.ID); rollbackError != nil {
			l.api.LogError("cannot rollback sender issue after send error, Err=", err.Error())
		}
		if rollbackError := l.store.RemoveIssue(receiverIssue.ID); rollbackError != nil {
			l.api.LogError("cannot rollback receiver issue after send error, Err=", err.Error())
		}
		return "", err
	}

	if err := l.store.AddReference(receiverID, receiverIssue.ID, InListKey, senderID, senderIssue.ID); err != nil {
		if rollbackError := l.store.RemoveIssue(senderIssue.ID); rollbackError != nil {
			l.api.LogError("cannot rollback sender issue after send error, Err=", err.Error())
		}
		if rollbackError := l.store.RemoveIssue(receiverIssue.ID); rollbackError != nil {
			l.api.LogError("cannot rollback receiver issue after send error ,Err=", err.Error())
		}
		if rollbackError := l.store.RemoveReference(senderID, senderIssue.ID, OutListKey); rollbackError != nil {
			l.api.LogError("cannot rollback sender list after send error, Err=", err.Error())
		}
		return "", err
	}

	return receiverIssue.ID, nil
}

func (l *listManager) GetIssueList(userID, listID string) ([]*ExtendedIssue, error) {
	irs, err := l.store.GetList(userID, listID)
	if err != nil {
		return nil, err
	}

	extendedIssues := []*ExtendedIssue{}
	for _, ir := range irs {
		issue, err := l.store.GetIssue(ir.IssueID)
		if err != nil {
			continue
		}

		extendedIssue := l.extendIssueInfo(issue, ir)
		extendedIssues = append(extendedIssues, extendedIssue)
	}

	return extendedIssues, nil
}

func (l *listManager) GetAllList(userID string) (listsIssue *ListsIssue, err error) {
	inListIssue, err := l.GetIssueList(userID, InListKey)
	if err != nil {
		return nil, err
	}
	myListIssue, err := l.GetIssueList(userID, MyListKey)
	if err != nil {
		return nil, err
	}
	outListIssue, err := l.GetIssueList(userID, OutListKey)
	if err != nil {
		return nil, err
	}
	return &ListsIssue{
		In:  inListIssue,
		My:  myListIssue,
		Out: outListIssue,
	}, nil
}

func (l *listManager) CompleteIssue(userID, issueID string) (issue *Issue, foreignID string, listToUpdate string, err error) {
	issueList, ir, _ := l.store.GetIssueListAndReference(userID, issueID)
	if ir == nil {
		return nil, "", issueList, fmt.Errorf("cannot find element")
	}

	if err = l.store.RemoveReference(userID, issueID, issueList); err != nil {
		return nil, "", issueList, err
	}

	issue, err = l.store.GetAndRemoveIssue(issueID)
	if err != nil {
		l.api.LogError("cannot remove issue, Err=", err.Error())
	}

	if ir.ForeignUserID == "" {
		return issue, "", issueList, nil
	}

	err = l.store.RemoveReference(ir.ForeignUserID, ir.ForeignIssueID, OutListKey)
	if err != nil {
		l.api.LogError("cannot clean foreigner list after complete, Err=", err.Error())
	}

	issue, err = l.store.GetAndRemoveIssue(ir.ForeignIssueID)
	if err != nil {
		l.api.LogError("cannot clean foreigner issue after complete, Err=", err.Error())
	}

	return issue, ir.ForeignUserID, issueList, nil
}

func (l *listManager) EditIssue(userID, issueID, newMessage, newDescription string) (foreignUserID, list, oldMessage string, err error) {
	issue, err := l.store.GetIssue(issueID)
	if err != nil {
		return "", "", "", err
	}

	list, ir, _ := l.store.GetIssueListAndReference(userID, issueID)
	if ir == nil {
		return "", "", "", errors.New("reference not found")
	}

	if ir.ForeignIssueID != "" {
		foreignIssue, foreignErr := l.store.GetIssue(ir.ForeignIssueID)
		if foreignErr == nil {
			oldMessage = foreignIssue.Message
			foreignIssue.Message = newMessage
			foreignIssue.Description = newDescription
			foreignErr = l.store.SaveIssue(foreignIssue)
			if foreignErr != nil {
				l.api.LogError("cannot edit foreign issue after edit", "error", foreignErr.Error())
			}
			if foreignIssue.ListID != "" && l.linked != nil {
				l.linked.EditLinkedItem(foreignIssue.ListID, foreignIssue.ID, newMessage, newDescription)
			}
		}
	}

	issue.Message = newMessage
	issue.Description = newDescription
	err = l.store.SaveIssue(issue)
	if err != nil {
		return "", "", "", err
	}

	return ir.ForeignUserID, list, oldMessage, nil
}

func (l *listManager) ChangeAssignment(issueID string, userID string, sendTo string) (issue *Issue, oldOwner string, err error) {
	issue, err = l.store.GetIssue(issueID)
	if err != nil {
		return nil, "", err
	}

	list, ir, _ := l.store.GetIssueListAndReference(userID, issueID)
	if ir == nil {
		return nil, "", errors.New("reference not found")
	}

	if (list == InListKey) || (ir.ForeignIssueID != "" && list == MyListKey) {
		return nil, "", errors.New("trying to change the assignment of a todo not owned")
	}

	if ir.ForeignUserID != "" {
		// Remove reference from foreign user
		foreignList, foreignIR, _ := l.store.GetIssueListAndReference(ir.ForeignUserID, ir.ForeignIssueID)
		if foreignIR != nil {
			if err := l.store.RemoveReference(ir.ForeignUserID, ir.ForeignIssueID, foreignList); err != nil {
				return nil, "", err
			}
		} else if foreignIssue, foreignErr := l.store.GetIssue(ir.ForeignIssueID); foreignErr != nil || foreignIssue.ListID == "" {
			return nil, "", errors.New("reference not found")
		}

		foreignIssue, err := l.store.GetAndRemoveIssue(ir.ForeignIssueID)
		if err != nil {
			l.api.LogError("cannot remove issue", "err", err.Error())
		}
		// Accepted into one of their lists: it leaves it
		l.removeLinked(foreignIssue)
	}

	if userID == sendTo && list == OutListKey {
		if err := l.store.RemoveReference(userID, issueID, OutListKey); err != nil {
			return nil, "", err
		}

		if err := l.store.AddReference(userID, issueID, MyListKey, "", ""); err != nil {
			return nil, "", err
		}

		return issue, ir.ForeignUserID, nil
	}

	if userID != sendTo {
		if err := l.store.RemoveReference(userID, issueID, list); err != nil {
			return nil, "", err
		}
	}

	receiverIssue := newIssue(issue.Message, issue.PostPermalink, issue.Description, issue.PostID)
	if err := l.store.SaveIssue(receiverIssue); err != nil {
		return nil, "", err
	}

	if err := l.store.AddReference(userID, issueID, OutListKey, sendTo, receiverIssue.ID); err != nil {
		return nil, "", err
	}

	if err := l.store.AddReference(sendTo, receiverIssue.ID, InListKey, userID, issue.ID); err != nil {
		return nil, "", err
	}

	return issue, ir.ForeignUserID, nil
}

func (l *listManager) GetRequest(userID, issueID string) (*Issue, *IssueRef, error) {
	ir, _, err := l.store.GetIssueReference(userID, issueID, InListKey)
	if err != nil {
		return nil, nil, err
	}
	issue, err := l.store.GetIssue(issueID)
	if err != nil {
		return nil, nil, err
	}
	return issue, ir, nil
}

func (l *listManager) LinkRequest(userID, issueID, listID string) error {
	issue, err := l.store.GetIssue(issueID)
	if err != nil {
		return err
	}
	issue.ListID = listID
	if err = l.store.SaveIssue(issue); err != nil {
		return err
	}
	return l.store.RemoveReference(userID, issueID, InListKey)
}

func (l *listManager) ReleaseRequest(issueID, senderID, senderIssueID string) error {
	if err := l.store.RemoveIssue(issueID); err != nil {
		l.api.LogError("cannot remove the receiver's copy of a todo", "err", err.Error())
	}
	if err := l.store.RemoveReference(senderID, senderIssueID, OutListKey); err != nil {
		l.api.LogError("cannot clean the sender's list", "err", err.Error())
	}
	return l.store.RemoveIssue(senderIssueID)
}

func (l *listManager) UpdateRequestText(senderIssueID, issueID, message, description string) (string, error) {
	if issue, err := l.store.GetIssue(issueID); err == nil {
		issue.Message, issue.Description = message, description
		if err = l.store.SaveIssue(issue); err != nil {
			l.api.LogError("cannot edit the receiver's copy of a todo", "err", err.Error())
		}
	}
	sender, err := l.store.GetIssue(senderIssueID)
	if err != nil {
		return "", err
	}
	old := sender.Message
	sender.Message, sender.Description = message, description
	return old, l.store.SaveIssue(sender)
}

func (l *listManager) RemoveIssue(userID, issueID string) (outIssue *Issue, foreignID string, isSender bool, listToUpdate string, outErr error) {
	issueList, ir, _ := l.store.GetIssueListAndReference(userID, issueID)
	if ir == nil {
		return nil, "", false, issueList, fmt.Errorf("cannot find element")
	}

	if err := l.store.RemoveReference(userID, issueID, issueList); err != nil {
		return nil, "", false, issueList, err
	}

	issue, err := l.store.GetAndRemoveIssue(issueID)
	if err != nil {
		l.api.LogError("cannot remove issue, Err=", err.Error())
	}

	if ir.ForeignUserID == "" {
		return issue, "", false, issueList, nil
	}

	list, _, _ := l.store.GetIssueListAndReference(ir.ForeignUserID, ir.ForeignIssueID)

	err = l.store.RemoveReference(ir.ForeignUserID, ir.ForeignIssueID, list)
	if err != nil {
		l.api.LogError("cannot clean foreigner list after remove, Err=", err.Error())
	}

	issue, err = l.store.GetAndRemoveIssue(ir.ForeignIssueID)
	if err != nil {
		l.api.LogError("cannot clean foreigner issue after remove, Err=", err.Error())
	}
	// Accepted into one of their lists: it leaves it
	l.removeLinked(issue)

	return issue, ir.ForeignUserID, list == OutListKey, issueList, nil
}

func (l *listManager) BumpIssue(userID, issueID string) (todo *Issue, receiver string, foreignIssueID string, outErr error) {
	ir, _, err := l.store.GetIssueReference(userID, issueID, OutListKey)
	if err != nil {
		return nil, "", "", err
	}

	if ir == nil {
		return nil, "", "", fmt.Errorf("cannot find sender issue")
	}

	err = l.store.BumpReference(ir.ForeignUserID, ir.ForeignIssueID, InListKey)
	if err != nil {
		return nil, "", "", err
	}

	issue, err := l.store.GetIssue(ir.ForeignIssueID)
	if err != nil {
		l.api.LogError("cannot find foreigner issue after bump, Err=", err.Error())
		return nil, "", "", nil
	}

	return issue, ir.ForeignUserID, ir.ForeignIssueID, nil
}

func (l *listManager) GetUserName(userID string) string {
	user, err := l.api.GetUser(userID)
	if err != nil {
		return "Someone"
	}
	return user.Username
}

func (l *listManager) extendIssueInfo(issue *Issue, ir *IssueRef) *ExtendedIssue {
	if issue == nil || ir == nil {
		return nil
	}

	feIssue := &ExtendedIssue{
		Issue: *issue,
	}

	if ir.ForeignUserID == "" {
		return feIssue
	}

	list, _, n := l.store.GetIssueListAndReference(ir.ForeignUserID, ir.ForeignIssueID)

	var listName string
	switch list {
	case MyListKey:
		listName = MyListKey
	case InListKey:
		listName = InFlag
	case OutListKey:
		listName = OutFlag
	}

	userName := l.GetUserName(ir.ForeignUserID)

	feIssue.ForeignUser = userName
	feIssue.ForeignUserID = ir.ForeignUserID
	feIssue.ForeignList = listName
	feIssue.ForeignPosition = n

	return feIssue
}
