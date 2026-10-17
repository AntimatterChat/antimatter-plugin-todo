package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSetttingsCommand(t *testing.T) {
	api := &plugintest.API{}
	api.On("SendEphemeralPost", mock.AnythingOfType("string"), mock.Anything).Return(nil)
	api.On("KVSet", mock.AnythingOfType("string"), mock.Anything).Return(nil)
	api.On("KVGet", mock.AnythingOfType("string"), mock.Anything).Return([]byte("true"), nil)

	apiKVSetFailed := &plugintest.API{}
	apiKVSetFailed.On("SendEphemeralPost", mock.AnythingOfType("string"), mock.Anything).Return(nil)
	apiKVSetFailed.On("KVSet", mock.AnythingOfType("string"), mock.Anything).Return(model.NewAppError("failed", "", nil, "", 400))
	apiKVSetFailed.On("LogDebug", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"))

	tests := []struct {
		name    string
		api     *plugintest.API
		args    []string
		wantErr bool
		want    bool
	}{
		{
			name:    "Setting successful without any arguments",
			api:     api,
			args:    []string{},
			wantErr: false,
			want:    false,
		},
		{
			name:    "Setting summary successful",
			api:     api,
			args:    []string{"summary", "on"},
			wantErr: false,
			want:    false,
		},
		{
			name:    "Setting summary failed due to KVSet failed",
			api:     apiKVSetFailed,
			args:    []string{"summary", "on"},
			wantErr: true,
			want:    false,
		},
		{
			name:    "Setting summary failed due to invalid number of arguments",
			api:     api,
			args:    []string{"summary", "on", "extraValue"},
			wantErr: true,
			want:    true,
		},
		{
			name:    "Setting summary successful due to no arguments",
			api:     api,
			args:    []string{"summary"},
			wantErr: false,
			want:    false,
		},
		{
			name:    "Setting summary failed due to invalid argument",
			api:     api,
			args:    []string{"summary", "test"},
			wantErr: true,
			want:    true,
		},
		{
			name:    "Setting allow_incoming_task_requests successful",
			api:     api,
			args:    []string{"allow_incoming_task_requests", "on"},
			wantErr: false,
			want:    false,
		},
		{
			name:    "Setting allow_incoming_task_requests failed due to KVSet failed",
			api:     apiKVSetFailed,
			args:    []string{"allow_incoming_task_requests", "on"},
			wantErr: true,
			want:    false,
		},
		{
			name:    "Setting allow_incoming_task_requests failed due to invalid number of arguments",
			api:     api,
			args:    []string{"allow_incoming_task_requests", "on", "extraValue"},
			wantErr: true,
			want:    true,
		},
		{
			name:    "Setting allow_incoming_task_requests successful due to no arguments",
			api:     api,
			args:    []string{"allow_incoming_task_requests"},
			wantErr: false,
			want:    false,
		},
		{
			name:    "Setting allow_incoming_task_requests failed due to invalid argument",
			api:     api,
			args:    []string{"allow_incoming_task_requests", "test"},
			wantErr: true,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := Plugin{}
			plugin.SetAPI(tt.api)

			resp, err := plugin.runSettingsCommand(tt.args, &model.CommandArgs{})
			if tt.wantErr != (err != nil) {
				t.Errorf("runSettingsCommand wantErr= %v got err= %v", tt.wantErr, err)
				return
			}

			if tt.want != resp {
				t.Errorf("runSettingsCommand got= %v want= %v", resp, tt.want)
			}
		})
	}
}

// run runs a /todo command as someone in a channel, and returns the bot's answer.
func run(t *testing.T, p *Plugin, userID, channelID, command string) string {
	t.Helper()
	api := p.API.(*fakeAPI)
	before := len(api.ephemeral[userID])
	_, appErr := p.ExecuteCommand(nil, &model.CommandArgs{UserId: userID, ChannelId: channelID, Command: command})
	require.Nil(t, appErr)
	answers := api.ephemeral[userID]
	require.Greater(t, len(answers), before, "an answer to %q", command)
	return answers[len(answers)-1]
}

func TestNewAndListsCommands(t *testing.T) {
	p, _ := newTestPlugin(t)
	answer := run(t, p, alice, town, "/todo new Launch checklist")
	assert.Contains(t, answer, "Made the list **Launch checklist**, shared with ~town-square")
	answer = run(t, p, alice, town, "/todo new --personal Groceries")
	assert.Contains(t, answer, "**Groceries**, only you can see it")
	answer = run(t, p, reader, news, "/todo new Headlines")
	assert.Contains(t, answer, "Error", "who can't post in a channel doesn't make its lists")
	answer = run(t, p, alice, town, "/todo new   ")
	assert.Contains(t, answer, "give the list a name")

	answer = run(t, p, alice, town, "/todo lists")
	assert.Contains(t, answer, "Todo lists of ~town-square")
	assert.Contains(t, answer, "**Launch checklist** (0 open)")
	assert.Contains(t, answer, "#### Your lists")
	assert.Contains(t, answer, "**My todos**")
	assert.Contains(t, answer, "**Groceries**")
	answer = run(t, p, bob, town, "/todo lists")
	assert.NotContains(t, answer, "Groceries", "nobody else's own lists")
	answer = run(t, p, outsider, town, "/todo lists")
	assert.NotContains(t, answer, "Launch checklist", "nor the lists of a channel one isn't in")
}

func TestAddCommandChoosesTheList(t *testing.T) {
	p, _ := newTestPlugin(t)
	answer := run(t, p, alice, town, "/todo add Water the plants")
	assert.Contains(t, answer, "Added to **My todos** (only you can see it)", "no list in the channel: my todos")

	launch := newList(t, p, alice, town, "Launch")
	groceries := newList(t, p, alice, "", "Groceries")
	answer = run(t, p, alice, town, "/todo add groceries: Oat milk")
	assert.Contains(t, answer, "Added to **Groceries**", "a list named before a colon, whatever the case")
	assert.Equal(t, []string{"Oat milk"}, messages(t, p, groceries.ID))

	answer = run(t, p, alice, town, "/todo add Note: the room is booked")
	assert.Contains(t, answer, "Added to **Groceries**", "no list is named Note: the last list used from this channel")
	assert.Equal(t, []string{"Oat milk", "Note: the room is booked"}, messages(t, p, groceries.ID), "with the whole text")

	answer = run(t, p, alice, town, "/todo add Launch: Book the room")
	assert.Contains(t, answer, "Added to **Launch** (shared with ~town-square)", "a channel's list when named")
	assert.Equal(t, []string{"Book the room"}, messages(t, p, launch.ID))

	answer = run(t, p, bob, town, "/todo add Groceries: steal alice's list")
	assert.NotContains(t, answer, "Groceries**", "nobody adds to someone else's list by naming it")
	assert.Equal(t, []string{"Oat milk", "Note: the room is booked"}, messages(t, p, groceries.ID))
}

func TestListAndPopCommands(t *testing.T) {
	p, api := newTestPlugin(t)
	mine, err := p.ensurePersonal(alice)
	require.NoError(t, err)
	addItem(t, p, alice, mine.ID, "First")
	_, err = p.AddItem(alice, mine.ID, itemRequest{Message: str("Second"), Due: str("2000-01-01")})
	require.NoError(t, err)

	answer := run(t, p, alice, town, "/todo list")
	assert.Contains(t, answer, "**My todos**")
	assert.Contains(t, answer, "- [ ] First")
	assert.Contains(t, answer, "- [ ] Second (overdue since 2000-01-01)")

	answer = run(t, p, alice, town, "/todo pop")
	assert.Contains(t, answer, "Removed the first todo of **My todos**")
	assert.Equal(t, []string{"Second"}, messages(t, p, mine.ID))
	run(t, p, alice, town, "/todo pop")
	answer = run(t, p, alice, town, "/todo pop")
	assert.Equal(t, "There are no Todos to pop.", answer)
	assert.Empty(t, api.replies)
}

// Privacy: without a list's name, "/todo add" never puts a todo in a list others see, even when the
// channel has lists, or the last list used from the channel is one of them.
func TestAddCommandWithoutAListStaysPersonal(t *testing.T) {
	p, _ := newTestPlugin(t)
	launch := newList(t, p, alice, town, "Launch")
	_, err := p.AddItem(bob, launch.ID, itemRequest{Message: str("Shared on purpose"), ChannelID: town})
	require.NoError(t, err)
	targets, err := p.Targets(bob, town)
	require.NoError(t, err)
	require.Equal(t, launch.ID, targets.DefaultID, "the message's Add to todo still offers it, in its picker")

	answer := run(t, p, bob, town, "/todo add Book a dentist")
	assert.Contains(t, answer, "Added to **My todos** (only you can see it)")
	assert.Equal(t, []string{"Shared on purpose"}, messages(t, p, launch.ID))
	assert.Equal(t, []string{"Book a dentist"}, messages(t, p, defaultPersonalListID(bob)))

	answer = run(t, p, carol, town, "/todo add Order the cake")
	assert.Contains(t, answer, "Added to **My todos**", "not the channel's first list either")
	assert.Equal(t, []string{"Shared on purpose"}, messages(t, p, launch.ID))
}
