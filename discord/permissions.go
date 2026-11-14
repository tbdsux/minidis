package discord

// Permission bits, for fields such as DefaultMemberPermissions:
//
//	DefaultMemberPermissions: discord.Int64(discord.PermissionManageGuild)
//
// Combine several with |.
const (
	PermissionCreateInstantInvite              int64 = 1 << 0
	PermissionKickMembers                      int64 = 1 << 1
	PermissionBanMembers                       int64 = 1 << 2
	PermissionAdministrator                    int64 = 1 << 3
	PermissionManageChannels                   int64 = 1 << 4
	PermissionManageGuild                      int64 = 1 << 5
	PermissionAddReactions                     int64 = 1 << 6
	PermissionViewAuditLog                     int64 = 1 << 7
	PermissionPrioritySpeaker                  int64 = 1 << 8
	PermissionStream                           int64 = 1 << 9
	PermissionViewChannel                      int64 = 1 << 10
	PermissionSendMessages                     int64 = 1 << 11
	PermissionSendTTSMessages                  int64 = 1 << 12
	PermissionManageMessages                   int64 = 1 << 13
	PermissionEmbedLinks                       int64 = 1 << 14
	PermissionAttachFiles                      int64 = 1 << 15
	PermissionReadMessageHistory               int64 = 1 << 16
	PermissionMentionEveryone                  int64 = 1 << 17
	PermissionUseExternalEmojis                int64 = 1 << 18
	PermissionViewGuildInsights                int64 = 1 << 19
	PermissionConnect                          int64 = 1 << 20
	PermissionSpeak                            int64 = 1 << 21
	PermissionMuteMembers                      int64 = 1 << 22
	PermissionDeafenMembers                    int64 = 1 << 23
	PermissionMoveMembers                      int64 = 1 << 24
	PermissionUseVAD                           int64 = 1 << 25
	PermissionChangeNickname                   int64 = 1 << 26
	PermissionManageNicknames                  int64 = 1 << 27
	PermissionManageRoles                      int64 = 1 << 28
	PermissionManageWebhooks                   int64 = 1 << 29
	PermissionManageGuildExpressions           int64 = 1 << 30
	PermissionUseApplicationCommands           int64 = 1 << 31
	PermissionRequestToSpeak                   int64 = 1 << 32
	PermissionManageEvents                     int64 = 1 << 33
	PermissionManageThreads                    int64 = 1 << 34
	PermissionCreatePublicThreads              int64 = 1 << 35
	PermissionCreatePrivateThreads             int64 = 1 << 36
	PermissionUseExternalStickers              int64 = 1 << 37
	PermissionSendMessagesInThreads            int64 = 1 << 38
	PermissionUseEmbeddedActivities            int64 = 1 << 39
	PermissionModerateMembers                  int64 = 1 << 40
	PermissionViewCreatorMonetizationAnalytics int64 = 1 << 41
	PermissionUseSoundboard                    int64 = 1 << 42
	PermissionCreateGuildExpressions           int64 = 1 << 43
	PermissionCreateEvents                     int64 = 1 << 44
	PermissionUseExternalSounds                int64 = 1 << 45
	PermissionSendVoiceMessages                int64 = 1 << 46
	PermissionSendPolls                        int64 = 1 << 49
	PermissionUseExternalApps                  int64 = 1 << 50
)
