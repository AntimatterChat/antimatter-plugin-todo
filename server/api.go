// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"net/http"

	"github.com/gorilla/mux"
)

// The lists' API, under /api/v1. Every route acts as the person of the request
// (Mattermost-User-ID, set by the server), with their rights (access.go).
//
//	GET    /lists?scope=mine|favorites           someone's own or favorite lists
//	GET    /lists?scope=channel&channel_id=…     a channel's lists
//	POST   /lists                                {name, icon, channel_id}: a list
//	GET    /lists/{list}                         a list and its todos
//	PATCH  /lists/{list}                         {name, icon, archived, position}
//	DELETE /lists/{list}
//	PUT    /lists/{list}/favorite                and DELETE
//	POST   /lists/{list}/items                   {message, description, assignee_id, due, post_id, channel_id}
//	PATCH  /lists/{list}/items/{item}            {message, description, assignee_id, due, done, position}
//	DELETE /lists/{list}/items/{item}
//	POST   /lists/{list}/items/{item}/send       {username}: a personal list's todo, to someone
//	GET    /targets?channel_id=…                 the lists to add a todo to from a channel
//	GET    /assigned                             the todos assigned to someone

func (p *Plugin) initListsAPI(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()
	list := "/lists/{list_id:[a-z0-9]{26}}"
	item := list + "/items/{item_id:[a-z0-9]{26}}"
	api.HandleFunc("/lists", p.checkAuth(p.handleGetLists)).Methods(http.MethodGet)
	api.HandleFunc("/lists", p.checkAuth(p.handleCreateList)).Methods(http.MethodPost)
	api.HandleFunc(list, p.checkAuth(p.handleGetList)).Methods(http.MethodGet)
	api.HandleFunc(list, p.checkAuth(p.handleUpdateList)).Methods(http.MethodPatch)
	api.HandleFunc(list, p.checkAuth(p.handleDeleteList)).Methods(http.MethodDelete)
	api.HandleFunc(list+"/favorite", p.checkAuth(p.handleFavorite)).Methods(http.MethodPut, http.MethodDelete)
	api.HandleFunc(list+"/items", p.checkAuth(p.handleAddItem)).Methods(http.MethodPost)
	api.HandleFunc(item, p.checkAuth(p.handleUpdateItem)).Methods(http.MethodPatch)
	api.HandleFunc(item, p.checkAuth(p.handleDeleteItem)).Methods(http.MethodDelete)
	api.HandleFunc(item+"/send", p.checkAuth(p.handleSendItem)).Methods(http.MethodPost)
	api.HandleFunc("/targets", p.checkAuth(p.handleTargets)).Methods(http.MethodGet)
	api.HandleFunc("/assigned", p.checkAuth(p.handleAssigned)).Methods(http.MethodGet)
}

func userOf(r *http.Request) string { return r.Header.Get("Mattermost-User-ID") }

func (p *Plugin) handleGetLists(w http.ResponseWriter, r *http.Request) {
	userID := userOf(r)
	var lists []ListInfo
	var err error
	switch r.URL.Query().Get("scope") {
	case "", "mine":
		lists, err = p.PersonalLists(userID)
	case "favorites":
		lists, err = p.FavoriteLists(userID)
	case ScopeChannel:
		channelID := r.URL.Query().Get("channel_id")
		if len(channelID) != 26 {
			err = newAPIError(http.StatusBadRequest, "which channel?")
			break
		}
		lists, err = p.ChannelLists(userID, channelID)
	default:
		err = newAPIError(http.StatusBadRequest, "unknown scope")
	}
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

func (p *Plugin) handleCreateList(w http.ResponseWriter, r *http.Request) {
	var req listRequest
	err := decodeBody(w, r, &req)
	if err == nil {
		var list *ListDetail
		if list, err = p.CreateList(userOf(r), req); err == nil {
			writeJSON(w, http.StatusCreated, list)
			return
		}
	}
	writeAPIError(w, err)
}

func (p *Plugin) handleGetList(w http.ResponseWriter, r *http.Request) {
	list, err := p.GetList(userOf(r), mux.Vars(r)["list_id"])
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (p *Plugin) handleUpdateList(w http.ResponseWriter, r *http.Request) {
	var req listRequest
	err := decodeBody(w, r, &req)
	if err == nil {
		var list *ListDetail
		if list, err = p.UpdateList(userOf(r), mux.Vars(r)["list_id"], req); err == nil {
			writeJSON(w, http.StatusOK, list)
			return
		}
	}
	writeAPIError(w, err)
}

func (p *Plugin) handleDeleteList(w http.ResponseWriter, r *http.Request) {
	if err := p.DeleteList(userOf(r), mux.Vars(r)["list_id"]); err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}

func (p *Plugin) handleFavorite(w http.ResponseWriter, r *http.Request) {
	favorite := r.Method == http.MethodPut
	if err := p.SetFavorite(userOf(r), mux.Vars(r)["list_id"], favorite); err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"favorite": favorite})
}

func (p *Plugin) handleAddItem(w http.ResponseWriter, r *http.Request) {
	var req itemRequest
	err := decodeBody(w, r, &req)
	if err == nil {
		var item *Item
		if item, err = p.AddItem(userOf(r), mux.Vars(r)["list_id"], req); err == nil {
			writeJSON(w, http.StatusCreated, item)
			return
		}
	}
	writeAPIError(w, err)
}

func (p *Plugin) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	var req itemRequest
	err := decodeBody(w, r, &req)
	if err == nil {
		var item *Item
		vars := mux.Vars(r)
		if item, err = p.UpdateItem(userOf(r), vars["list_id"], vars["item_id"], req); err == nil {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	writeAPIError(w, err)
}

func (p *Plugin) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if err := p.DeleteItem(userOf(r), vars["list_id"], vars["item_id"]); err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}

func (p *Plugin) handleSendItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	err := decodeBody(w, r, &req)
	if err == nil {
		vars := mux.Vars(r)
		if _, err = p.SendItem(userOf(r), vars["list_id"], vars["item_id"], req.Username); err == nil {
			writeJSON(w, http.StatusOK, map[string]string{"status": "OK"})
			return
		}
	}
	writeAPIError(w, err)
}

func (p *Plugin) handleTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := p.Targets(userOf(r), r.URL.Query().Get("channel_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (p *Plugin) handleAssigned(w http.ResponseWriter, r *http.Request) {
	todos, err := p.AssignedTodos(userOf(r))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, todos)
}
