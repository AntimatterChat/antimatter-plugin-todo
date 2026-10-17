// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/pkg/errors"
)

// apiError is an error with the HTTP status to answer it with.
type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func newAPIError(status int, format string, args ...any) *apiError {
	return &apiError{status: status, msg: fmt.Sprintf(format, args...)}
}

// errorStatus is the HTTP status of an error: its own, or 500.
func errorStatus(err error) int {
	var a *apiError
	var appErr *model.AppError
	switch {
	case errors.As(err, &a):
		return a.status
	case errors.As(err, &appErr) && appErr.StatusCode != 0:
		return appErr.StatusCode
	}
	return http.StatusInternalServerError
}

// kvGetJSON reads a JSON value of the KV store into v, and tells whether it exists.
func kvGetJSON(api plugin.API, key string, v any) (bool, error) {
	data, appErr := api.KVGet(key)
	if appErr != nil {
		return false, appErr
	}
	if data == nil {
		return false, nil
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, errors.Wrapf(err, "cannot decode %s", key)
	}
	return true, nil
}

// kvCreateJSON writes a JSON value unless the key already has one, and tells whether it wrote it.
func kvCreateJSON(api plugin.API, key string, v any) (bool, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	ok, appErr := api.KVCompareAndSet(key, nil, data)
	if appErr != nil {
		return false, appErr
	}
	return ok, nil
}

// updateKV changes a JSON value of the KV store atomically: it reads it, applies change, and writes
// it back if nobody changed it meanwhile (retrying otherwise). Nothing is written when change leaves
// the value as it was.
func updateKV[T any](api plugin.API, key string, change func(*T) error) (*T, error) {
	for range StoreRetries + 2 {
		old, appErr := api.KVGet(key)
		if appErr != nil {
			return nil, appErr
		}
		var value T
		if old != nil {
			if err := json.Unmarshal(old, &value); err != nil {
				return nil, errors.Wrapf(err, "cannot decode %s", key)
			}
		}
		if err := change(&value); err != nil {
			return nil, err
		}
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(data, old) {
			return &value, nil
		}
		ok, appErr := api.KVCompareAndSet(key, old, data)
		if appErr != nil {
			return nil, appErr
		}
		if ok {
			return &value, nil
		}
	}
	return nil, errors.New("the list changed too often, try again")
}

// HTTP --------------------------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, err error) {
	writeJSON(w, errorStatus(err), map[string]string{"error": err.Error()})
}

// decodeBody reads a JSON request body of at most 64 KiB.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(v); err != nil {
		return newAPIError(http.StatusBadRequest, "invalid request")
	}
	return nil
}
