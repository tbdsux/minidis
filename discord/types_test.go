package discord

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestComponentsEncodeWithType(t *testing.T) {
	got, err := json.Marshal([]MessageComponent{
		ActionsRow{Components: []MessageComponent{
			Button{Label: "Yes", Style: SuccessButton, CustomID: "yes"},
			SelectMenu{CustomID: "s", Options: []SelectMenuOption{{Label: "A", Value: "a"}}},
			SelectMenu{MenuType: UserSelectMenuComponent, CustomID: "u"},
		}},
		Label{Label: "Name", Component: TextInput{CustomID: "n", Style: TextInputShort, Required: Bool(false)}},
		Separator{},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := `[
		{"type":1,"components":[
			{"type":2,"label":"Yes","style":3,"custom_id":"yes"},
			{"type":3,"custom_id":"s","options":[{"label":"A","value":"a"}]},
			{"type":5,"custom_id":"u"}
		]},
		{"type":18,"label":"Name","component":{"type":4,"custom_id":"n","style":1,"required":false}},
		{"type":14}
	]`

	assertJSONEqual(t, got, want)
}

func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()

	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("got invalid JSON %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestComponentsRoundTrip(t *testing.T) {
	input := `[
		{"type":1,"id":1,"components":[{"type":2,"style":5,"url":"https://x","label":"L"}]},
		{"type":9,"components":[{"type":10,"content":"hi"}],"accessory":{"type":11,"media":{"url":"attachment://a.png"}}},
		{"type":17,"components":[{"type":12,"items":[{"media":{"url":"u"}}]},{"type":13,"file":{"url":"attachment://f"}}],"accent_color":255},
		{"type":18,"label":"Up","component":{"type":19,"custom_id":"f","max_values":2}},
		{"type":99,"future":{"nested":true}}
	]`

	var list ComponentList
	if err := json.Unmarshal([]byte(input), &list); err != nil {
		t.Fatal(err)
	}

	if _, ok := list[0].(*ActionsRow).Components[0].(*Button); !ok {
		t.Errorf("row child = %T", list[0].(*ActionsRow).Components[0])
	}
	if _, ok := list[1].(*Section).Accessory.(*Thumbnail); !ok {
		t.Errorf("accessory = %T", list[1].(*Section).Accessory)
	}
	if _, ok := list[3].(*Label).Component.(*FileUpload); !ok {
		t.Errorf("label child = %T", list[3].(*Label).Component)
	}
	if u, ok := list[4].(*UnknownComponent); !ok || u.Type() != 99 {
		t.Errorf("unknown = %#v", list[4])
	}

	got, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, input)
}

func TestDecodeApplicationCommandInteraction(t *testing.T) {
	data := `{
		"id":"1","application_id":"2","type":2,"token":"t","version":1,"guild_id":"3","channel_id":"4",
		"app_permissions":"2147483648","locale":"en-US","guild_locale":"de","context":0,
		"member":{"user":{"id":"5","username":"u","global_name":null},"roles":["6"],"joined_at":"2021-01-02T03:04:05.123+00:00","permissions":"8","communication_disabled_until":null},
		"data":{"id":"7","name":"cmd","type":1,"target_id":"m1",
			"options":[{"name":"s","type":3,"value":"v"},{"name":"i","type":4,"value":12},{"name":"b","type":5,"value":true},{"name":"n","type":10,"value":1.5},{"name":"u","type":6,"value":"5"}],
			"resolved":{"users":{"5":{"id":"5"}},"members":{"5":{"nick":"n","roles":[],"joined_at":null,"permissions":"0"}},"messages":{"m1":{"id":"m1","content":"body","components":null}}}}
	}`

	var i Interaction
	if err := json.Unmarshal([]byte(data), &i); err != nil {
		t.Fatal(err)
	}

	if i.Member.User.ID != "5" || i.Member.Permissions != 8 || i.AppPermissions != 1<<31 || *i.GuildLocale != German || i.Locale != EnglishUS {
		t.Errorf("interaction = %+v", i)
	}

	d := i.ApplicationCommandData()
	if d.Name != "cmd" || d.CommandType != ChatApplicationCommand {
		t.Errorf("data = %+v", d)
	}
	if d.GetOption("s").StringValue() != "v" || d.GetOption("i").IntValue() != 12 || !d.GetOption("b").BoolValue() ||
		d.GetOption("n").FloatValue() != 1.5 || d.GetOption("u").IDValue() != "5" || d.GetOption("missing") != nil {
		t.Errorf("options = %+v", d.Options)
	}
	if d.Resolved.Messages[d.TargetID].Content != "body" || d.Resolved.Members["5"].Nick != "n" {
		t.Errorf("resolved = %+v", d.Resolved)
	}
}

func TestDecodeComponentAndModalInteractions(t *testing.T) {
	var c Interaction
	if err := json.Unmarshal([]byte(`{"type":3,"user":{"id":"1"},"app_permissions":"0","data":{"custom_id":"sel","component_type":3,"values":["go"]},
		"message":{"id":"m","components":[{"type":1,"components":[{"type":3,"custom_id":"sel","options":[]}]}]}}`), &c); err != nil {
		t.Fatal(err)
	}
	if d := c.MessageComponentData(); d.CustomID != "sel" || d.Values[0] != "go" || d.ComponentType != SelectMenuComponent {
		t.Errorf("component data = %+v", d)
	}
	if _, ok := c.Message.Components[0].(*ActionsRow); !ok {
		t.Errorf("message component = %T", c.Message.Components[0])
	}

	var m Interaction
	if err := json.Unmarshal([]byte(`{"type":5,"app_permissions":"0","data":{"custom_id":"modal","components":[
		{"type":18,"id":1,"component":{"type":4,"id":2,"custom_id":"a","value":"typed"}},
		{"type":1,"components":[{"type":4,"custom_id":"b","value":"legacy"}]}
	]}}`), &m); err != nil {
		t.Fatal(err)
	}

	d := m.ModalSubmitData()
	if got := d.Components[0].(*Label).Component.(*TextInput).Value; got != "typed" {
		t.Errorf("label value = %q", got)
	}
	if got := d.Components[1].(*ActionsRow).Components[0].(*TextInput).Value; got != "legacy" {
		t.Errorf("row value = %q", got)
	}
}

func TestPingHasNoData(t *testing.T) {
	var i Interaction
	if err := json.Unmarshal([]byte(`{"type":1,"id":"1","app_permissions":"0"}`), &i); err != nil {
		t.Fatal(err)
	}
	if i.Data != nil {
		t.Errorf("data = %v", i.Data)
	}
}

func TestWebhookEditEmptyContentIsSent(t *testing.T) {
	empty := ""
	got, err := json.Marshal(WebhookEdit{Content: &empty})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, `{"content":""}`)
}

func TestApplicationCommandPermissionsEncoding(t *testing.T) {
	zero := int64(0)
	got, err := json.Marshal(ApplicationCommand{Name: "x", DefaultMemberPermissions: &zero})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, `{"name":"x","default_member_permissions":"0"}`)

	var c ApplicationCommand
	if err := json.Unmarshal([]byte(`{"name":"x","default_member_permissions":null,"name_localizations":null,"contexts":null}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.DefaultMemberPermissions != nil {
		t.Errorf("permissions = %v", *c.DefaultMemberPermissions)
	}
}

func TestDecodeInteractionContextFields(t *testing.T) {
	var i Interaction
	if err := json.Unmarshal([]byte(`{"type":2,"id":"1","app_permissions":"0",
		"guild":{"id":"g","features":["COMMUNITY"],"locale":"en-US"},
		"channel":{"id":"c","type":0},
		"entitlements":[{"id":"e","sku_id":"s","application_id":"a","type":8,"deleted":false,"starts_at":"2026-01-02T03:04:05Z","ends_at":null,"consumed":true}],
		"authorizing_integration_owners":{"0":"g","1":"u"},
		"attachment_size_limit":10485760,
		"data":{"id":"cmd","name":"x","type":1}}`), &i); err != nil {
		t.Fatal(err)
	}

	if i.Guild.ID != "g" || i.Channel.ID != "c" || i.AttachmentSizeLimit != 10485760 {
		t.Errorf("interaction = %+v", i)
	}
	owners := i.AuthorizingIntegrationOwners
	if owners[ApplicationIntegrationGuildInstall] != "g" || owners[ApplicationIntegrationUserInstall] != "u" {
		t.Errorf("owners = %v", owners)
	}
	e := i.Entitlements[0]
	if e.SKUID != "s" || e.Type != EntitlementApplicationSubscription || e.StartsAt == nil || e.EndsAt != nil || e.Consumed == nil || !*e.Consumed {
		t.Errorf("entitlement = %+v", e)
	}
}

func TestDecodeModalGroupComponents(t *testing.T) {
	var m Interaction
	if err := json.Unmarshal([]byte(`{"type":5,"app_permissions":"0","data":{"custom_id":"modal","components":[
		{"type":18,"component":{"type":21,"custom_id":"r","value":"b"}},
		{"type":18,"component":{"type":21,"custom_id":"r0","value":null}},
		{"type":18,"component":{"type":22,"custom_id":"g","values":["x","y"]}},
		{"type":18,"component":{"type":23,"custom_id":"c","value":true}}
	]}}`), &m); err != nil {
		t.Fatal(err)
	}

	child := func(i int) MessageComponent { return m.ModalSubmitData().Components[i].(*Label).Component }

	if r := child(0).(*RadioGroup); r.CustomID != "r" || r.Value == nil || *r.Value != "b" {
		t.Errorf("radio = %+v", r)
	}
	if r := child(1).(*RadioGroup); r.Value != nil {
		t.Errorf("unpicked radio = %q", *r.Value)
	}
	if g := child(2).(*CheckboxGroup); !reflect.DeepEqual(g.Values, []string{"x", "y"}) {
		t.Errorf("group = %+v", g)
	}
	if c := child(3).(*Checkbox); !c.Value {
		t.Errorf("checkbox = %+v", c)
	}
}

func TestGroupComponentsEncode(t *testing.T) {
	opts := []GroupOption{{Value: "a", Label: "A", Default: true}, {Value: "b", Label: "B", Description: "d"}}
	got, err := json.Marshal([]MessageComponent{
		RadioGroup{CustomID: "r", Options: opts, Required: Bool(false)},
		CheckboxGroup{CustomID: "g", Options: opts, MinValues: Int(0), MaxValues: 2},
		Checkbox{CustomID: "c", Default: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	options := `[{"value":"a","label":"A","default":true},{"value":"b","label":"B","description":"d"}]`
	assertJSONEqual(t, got, `[
		{"type":21,"custom_id":"r","options":`+options+`,"required":false},
		{"type":22,"custom_id":"g","options":`+options+`,"min_values":0,"max_values":2},
		{"type":23,"custom_id":"c","default":true}
	]`)
}

func TestPollEncodes(t *testing.T) {
	got, err := json.Marshal(InteractionResponseData{Poll: &PollCreateRequest{
		Question: PollMedia{Text: "Best?"},
		Answers:  []PollAnswer{{PollMedia: PollMedia{Text: "Go", Emoji: &ComponentEmoji{Name: "🦦"}}}},
		Duration: 2,
	}})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, `{"poll":{"question":{"text":"Best?"},"answers":[{"poll_media":{"text":"Go","emoji":{"name":"🦦"}}}],"duration":2}}`)
}

func TestDecodeMessagePoll(t *testing.T) {
	var m Message
	if err := json.Unmarshal([]byte(`{"id":"m","poll":{"question":{"text":"Q"},"answers":[{"answer_id":1,"poll_media":{"text":"A"}}],
		"expiry":"2026-01-02T03:04:05Z","allow_multiselect":false,"layout_type":1,
		"results":{"is_finalized":true,"answer_counts":[{"id":1,"count":3,"me_voted":false}]}}}`), &m); err != nil {
		t.Fatal(err)
	}
	if p := m.Poll; p.Answers[0].AnswerID != 1 || p.Expiry == nil || !p.Results.IsFinalized || p.Results.AnswerCounts[0].Count != 3 {
		t.Errorf("poll = %+v", p)
	}
}

func TestWebhookEditFlags(t *testing.T) {
	v2, none := MessageFlagsIsComponentsV2, MessageFlags(0)

	tests := []struct {
		name string
		edit any
		want string
	}{
		{"unset flags are left out", WebhookEdit{}, `{}`},
		{"zero clears the flags", WebhookEdit{Flags: &none}, `{"flags":0}`},
		{"components v2 nulls content and poll", WebhookEdit{Flags: &v2}, `{"flags":32768,"content":null,"poll":null}`},
		{"components v2 keeps a set content", WebhookEdit{Flags: &v2, Content: String("x")}, `{"flags":32768,"content":"x","poll":null}`},
		{"response data does the same", InteractionResponseData{Flags: v2}, `{"flags":32768,"content":null,"poll":null}`},
		{"other response data is unchanged", InteractionResponseData{Flags: MessageFlagsEphemeral}, `{"flags":64}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.edit)
			if err != nil {
				t.Fatal(err)
			}
			assertJSONEqual(t, got, tt.want)
		})
	}
}

func TestApplicationCommandEditLeavesNameOut(t *testing.T) {
	got, err := json.Marshal(ApplicationCommand{Description: "new"})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, `{"description":"new"}`)
}

func TestEntryPointCommandEncodes(t *testing.T) {
	got, err := json.Marshal(ApplicationCommand{Name: "launch", Type: PrimaryEntryPointApplicationCommand, Handler: EntryPointHandlerDiscordLaunchActivity})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, `{"name":"launch","type":4,"handler":2}`)
}
