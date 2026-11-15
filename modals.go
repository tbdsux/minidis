package minidis

import (
	"errors"
	"fmt"

	"github.com/tbdsux/minidis/discord"
)

// ModalSubmitContext is passed to the handler of a submitted modal. Read the
// values with Text, Selected and Checked.
type ModalSubmitContext struct {
	*Context
	Data discord.ModalSubmitInteractionData

	// Params holds the captures of a handler ID such as "feedback:{id}".
	Params map[string]string
}

// HandleModal sets the handler for submissions of the modals with the custom ID
// id. The id can contain captures, as described on HandleComponent.
func (m *Minidis) HandleModal(id string, h func(c *ModalSubmitContext) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.modals.add(id, h)
}

// HandleModalFallback sets the handler for modal submissions that no
// HandleModal id matches. Without one, they reach the error handler as
// ErrNoHandler.
func (m *Minidis) HandleModalFallback(h func(c *ModalSubmitContext) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.modalFallback = h
}

func (m *Minidis) routeModalSubmit(c *Context) HandlerFunc {
	data := c.Interaction.ModalSubmitData()

	h, params, ok := m.modals.find(data.CustomID)
	if !ok {
		h = m.modalFallback
	}
	if h == nil {
		return failWith(fmt.Errorf("%w: modal %q", ErrNoHandler, data.CustomID))
	}

	return func(c *Context) error {
		return h(&ModalSubmitContext{Context: c, Data: data, Params: params})
	}
}

// Update responds by replacing the text of the message whose component opened
// the modal. A modal opened by a command has no such message.
func (c *ModalSubmitContext) Update(content string) error {
	return c.UpdateWith(MessageEdit{Content: &content})
}

// errNoModalMessage is returned by the update methods of a modal opened by a
// command, which has no message to update.
var errNoModalMessage = errors.New("minidis: the modal was not opened from a message component, so there is no message to update")

// UpdateWith responds by changing the message whose component opened the
// modal.
func (c *ModalSubmitContext) UpdateWith(edit MessageEdit) error {
	if c.Interaction.Message == nil {
		return errNoModalMessage
	}
	return c.update(edit)
}

// DeferUpdate acknowledges the submission without changing the message whose
// component opened the modal yet. Change it within 15 minutes using Edit.
func (c *ModalSubmitContext) DeferUpdate() error {
	if c.Interaction.Message == nil {
		return errNoModalMessage
	}
	return c.deferUpdate()
}

// Component returns the submitted component with the custom ID, such as a
// *discord.TextInput, or nil.
func (c *ModalSubmitContext) Component(customID string) discord.MessageComponent {
	return findComponent(c.Data.Components, customID)
}

func findComponent(list discord.ComponentList, customID string) discord.MessageComponent {
	for _, component := range list {
		var id string
		switch v := component.(type) {
		case *discord.ActionsRow:
			if found := findComponent(v.Components, customID); found != nil {
				return found
			}
			continue
		case *discord.Label:
			if found := findComponent(discord.ComponentList{v.Component}, customID); found != nil {
				return found
			}
			continue
		case *discord.TextInput:
			id = v.CustomID
		case *discord.SelectMenu:
			id = v.CustomID
		case *discord.FileUpload:
			id = v.CustomID
		case *discord.RadioGroup:
			id = v.CustomID
		case *discord.CheckboxGroup:
			id = v.CustomID
		case *discord.Checkbox:
			id = v.CustomID
		}
		if id != "" && id == customID {
			return component
		}
	}
	return nil
}

// Text returns the text entered in a text input or the option chosen in a
// radio group, or "" if there is none.
func (c *ModalSubmitContext) Text(customID string) string {
	switch v := c.Component(customID).(type) {
	case *discord.TextInput:
		return v.Value
	case *discord.RadioGroup:
		if v.Value != nil {
			return *v.Value
		}
	}
	return ""
}

// Selected returns the values chosen in a select menu or checkbox group, or the
// attachment IDs of a file upload. For user, role, mentionable and channel
// selects the values are IDs, whose objects are in c.Data.Resolved.
func (c *ModalSubmitContext) Selected(customID string) []string {
	switch v := c.Component(customID).(type) {
	case *discord.SelectMenu:
		return v.Values
	case *discord.CheckboxGroup:
		return v.Values
	case *discord.FileUpload:
		return v.Values
	}
	return nil
}

// Checked reports whether a checkbox was ticked.
func (c *ModalSubmitContext) Checked(customID string) bool {
	v, ok := c.Component(customID).(*discord.Checkbox)
	return ok && v.Value
}

// Attachments returns the files uploaded to a file upload.
func (c *ModalSubmitContext) Attachments(customID string) []*discord.MessageAttachment {
	ids := c.Selected(customID)
	if len(ids) == 0 || c.Data.Resolved == nil {
		return nil
	}

	files := make([]*discord.MessageAttachment, 0, len(ids))
	for _, id := range ids {
		if a := c.Data.Resolved.Attachments[id]; a != nil {
			files = append(files, a)
		}
	}
	return files
}
