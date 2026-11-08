package minidis

// ClearCommands removes the application commands from the given guilds.
// If no guilds are given, the global commands are removed instead.
func (m *Minidis) ClearCommands(guildIDs ...string) error {
	if len(guildIDs) == 0 {
		_, err := m.Session.ApplicationCommandBulkOverwrite(m.AppID, "", nil)
		return err
	}

	for _, v := range guildIDs {
		if _, err := m.Session.ApplicationCommandBulkOverwrite(m.AppID, v, nil); err != nil {
			return err
		}
	}

	return nil
}
