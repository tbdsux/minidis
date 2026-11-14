package minidis

import (
	"fmt"

	"github.com/tbdsux/minidis/discord"
)

// ModalSubmitContext is passed to the handler of a submitted modal.
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

// Value returns the text entered in a text input or the option chosen in a
// radio group, or "" if there is none.
func (c *ModalSubmitContext) Value(customID string) string {
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

// Values returns the values chosen in a select menu or checkbox group, or the
// attachment IDs of a file upload. For user, role, mentionable and channel
// selects the values are IDs, whose objects are in c.Data.Resolved.
func (c *ModalSubmitContext) Values(customID string) []string {
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
	ids := c.Values(customID)
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
