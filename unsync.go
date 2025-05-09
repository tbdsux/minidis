package minidis

// ClearCommands removes the application commands from the guild.
// If there are no guilds specified using `SyncToGuilds()`, global commands will be removed.
func (m *Minidis) ClearCommands(guildIDs ...string) error {
	for _, v := range guildIDs {
		// get application commands
		guildCommands, err := m.Session.ApplicationCommands(m.AppID, v)
		if err != nil {
			return err
		}

		for _, cmd := range guildCommands {
			if err = m.Session.ApplicationCommandDelete(m.AppID, v, cmd.ID); err != nil {
				return err
			}
		}
	}

	// Remove global commands
	globalCommands, err := m.Session.ApplicationCommands(m.AppID, "")
	if err != nil {
		return err
	}

	for _, cmd := range globalCommands {
		if err = m.Session.ApplicationCommandDelete(m.AppID, "", cmd.ID); err != nil {
			return err
		}
	}

	return nil
}
