package gamesettings

// Option describes one PalWorldSettings.ini setting for the editor. Defaults are the game's
// DefaultPalWorldSettings.ini values; options the catalog doesn't know are still shown and kept.
type Option struct {
	Key     string   `json:"key"`
	Group   string   `json:"group"`
	Label   string   `json:"label"`
	Help    string   `json:"help"`
	Type    string   `json:"type"` // float, int, bool, enum, string, secret
	Default string   `json:"default"`
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	Choices []string `json:"choices,omitempty"`
	Locked  bool     `json:"locked,omitempty"` // shown, not editable here (Warden depends on it)
}

func f(group, key, label, help, def string, min, max float64) Option {
	return Option{Key: key, Group: group, Label: label, Help: help, Type: "float", Default: def, Min: min, Max: max}
}
func i(group, key, label, help, def string, min, max float64) Option {
	return Option{Key: key, Group: group, Label: label, Help: help, Type: "int", Default: def, Min: min, Max: max}
}
func b(group, key, label, help, def string) Option {
	return Option{Key: key, Group: group, Label: label, Help: help, Type: "bool", Default: def}
}

// Catalog lists the options Warden explains, in the order the editor shows them.
var Catalog = []Option{
	// The world
	f("World", "DayTimeSpeedRate", "Day speed", "How fast daytime passes. 2 = days are half as long.", "1.000000", 0.1, 5),
	f("World", "NightTimeSpeedRate", "Night speed", "How fast night passes. 2 = nights are half as long.", "1.000000", 0.1, 5),
	f("World", "ExpRate", "Experience", "Experience gained by players and Pals. 2 = twice as fast. Above the game menu's range (20) the server may cap it: the Server page shows what it really uses.", "1.000000", 0.1, 50),
	f("World", "PalCaptureRate", "Capture chance", "How easy Pals are to catch. 2 = twice the chance.", "1.000000", 0.5, 10),
	f("World", "PalSpawnNumRate", "Pal numbers", "How many wild Pals appear. Above 1 costs server performance.", "1.000000", 0.5, 3),
	f("World", "WorkSpeedRate", "Work speed", "How fast Pals work at bases.", "1.000000", 0.1, 10),
	f("World", "PalEggDefaultHatchingTime", "Egg hatching (hours)", "Hours for a huge egg to hatch; smaller eggs scale down.", "72.000000", 0, 240),
	f("World", "CollectionDropRate", "Gathering amount", "Items from trees, rocks and other gathering.", "1.000000", 0.5, 20),
	f("World", "CollectionObjectHpRate", "Gathering effort", "How sturdy trees and rocks are.", "1.000000", 0.5, 3),
	f("World", "CollectionObjectRespawnSpeedRate", "Regrowth time", "How long gathered trees and rocks take to come back. Lower = faster.", "1.000000", 0.5, 3),
	f("World", "EnemyDropItemRate", "Enemy drops", "Items dropped by defeated enemies.", "1.000000", 0.5, 20),
	i("World", "SupplyDropSpan", "Supply drops (minutes)", "Minutes between meteorite and supply drops.", "180", 1, 1440),
	b("World", "bEnableInvaderEnemy", "Base raids", "Whether raiders attack bases.", "True"),
	b("World", "bActiveUNKO", "Pal droppings", "Whether Pals leave droppings at bases.", "False"),
	// Players
	f("Players", "PlayerDamageRateAttack", "Player damage dealt", "Damage players deal.", "1.000000", 0.1, 5),
	f("Players", "PlayerDamageRateDefense", "Player damage taken", "Damage players take. Lower = tougher players.", "1.000000", 0.1, 5),
	f("Players", "PlayerStomachDecreaceRate", "Player hunger", "How fast players get hungry.", "1.000000", 0.1, 5),
	f("Players", "PlayerStaminaDecreaceRate", "Player stamina use", "How fast players use stamina.", "1.000000", 0.1, 5),
	f("Players", "PlayerAutoHPRegeneRate", "Player healing", "How fast players heal on their own.", "1.000000", 0.1, 5),
	f("Players", "PlayerAutoHpRegeneRateInSleep", "Player healing asleep", "How fast players heal while sleeping.", "1.000000", 0.1, 5),
	{Key: "DeathPenalty", Group: "Players", Label: "What players drop on death", Help: "None, items only, items and equipment, or everything including Pals in the party.", Type: "enum", Default: "All", Choices: []string{"None", "Item", "ItemAndEquipment", "All"}},
	b("Players", "bEnableFastTravel", "Fast travel", "Whether players can fast travel.", "True"),
	b("Players", "bIsStartLocationSelectByMap", "Choose start on the map", "New players pick where they start.", "True"),
	b("Players", "bExistPlayerAfterLogout", "Bodies stay after logout", "A logged-out player's body stays in the world (and can be attacked).", "False"),
	// Pals
	f("Pals", "PalDamageRateAttack", "Pal damage dealt", "Damage Pals deal.", "1.000000", 0.1, 5),
	f("Pals", "PalDamageRateDefense", "Pal damage taken", "Damage Pals take.", "1.000000", 0.1, 5),
	f("Pals", "PalStomachDecreaceRate", "Pal hunger", "How fast Pals get hungry.", "1.000000", 0.1, 5),
	f("Pals", "PalStaminaDecreaceRate", "Pal stamina use", "How fast Pals use stamina.", "1.000000", 0.1, 5),
	f("Pals", "PalAutoHPRegeneRate", "Pal healing", "How fast Pals heal on their own.", "1.000000", 0.1, 5),
	f("Pals", "PalAutoHpRegeneRateInSleep", "Pal healing in the Palbox", "How fast Pals heal while resting.", "1.000000", 0.1, 5),
	// Bases and guilds
	i("Bases", "BaseCampMaxNum", "Bases on the server", "The most bases the whole server can have. High numbers cost performance.", "128", 1, 500),
	i("Bases", "BaseCampWorkerMaxNum", "Workers per base", "The most Pals working at one base (the game allows up to 50).", "15", 1, 50),
	i("Bases", "BaseCampMaxNumInGuild", "Bases per guild", "How many bases one guild may have (up to 10).", "4", 1, 10),
	i("Bases", "GuildPlayerMaxNum", "Players per guild", "The most players in one guild.", "20", 1, 100),
	f("Bases", "BuildObjectDamageRate", "Building damage", "Damage buildings take.", "1.000000", 0, 5),
	f("Bases", "BuildObjectDeteriorationDamageRate", "Building decay", "How fast buildings outside a base wear down. 0 = never.", "1.000000", 0, 10),
	i("Bases", "DropItemMaxNum", "Items on the ground", "The most dropped items in the world at once. High numbers cost performance.", "3000", 100, 10000),
	f("Bases", "DropItemAliveMaxHours", "Dropped items last (hours)", "How long dropped items stay before they vanish.", "1.000000", 0.1, 24),
	b("Bases", "bAutoResetGuildNoOnlinePlayers", "Remove abandoned guilds", "Delete a guild's bases when none of its players has logged in for a while.", "False"),
	f("Bases", "AutoResetGuildTimeNoOnlinePlayers", "Abandoned after (hours)", "Hours without a guild member online before its bases are removed (if that's on).", "72.000000", 1, 2160),
	// Players together
	b("Multiplayer", "bIsPvP", "PvP", "Players can fight each other.", "False"),
	b("Multiplayer", "bEnablePlayerToPlayerDamage", "Player-on-player damage", "Players can hurt each other.", "False"),
	b("Multiplayer", "bEnableFriendlyFire", "Friendly fire", "Players can hurt their own guild.", "False"),
	b("Multiplayer", "bCanPickupOtherGuildDeathPenaltyDrop", "Loot other guilds' deaths", "Players can pick up what another guild's player dropped on death.", "False"),
	b("Multiplayer", "bEnableDefenseOtherGuildPlayer", "Bases defend against other guilds", "Base defences attack players from other guilds.", "False"),
	b("Multiplayer", "bEnableNonLoginPenalty", "Penalty for long absence", "Pals at a base suffer when their owner hasn't logged in for a long time.", "True"),
	i("Multiplayer", "CoopPlayerMaxNum", "Party size", "The most players in one party.", "4", 1, 32),
	// The server
	{Key: "ServerName", Group: "Server", Label: "Server name", Help: "Shown in the server list.", Type: "string", Default: "Default Palworld Server"},
	{Key: "ServerDescription", Group: "Server", Label: "Description", Help: "Shown in the server list.", Type: "string"},
	{Key: "ServerPassword", Group: "Server", Label: "Join password", Help: "Players need it to join. Empty = anyone can join.", Type: "secret"},
	i("Server", "ServerPlayerMaxNum", "Player slots", "The most players online at once (the game allows up to 32).", "32", 1, 32),
	b("Server", "bShowPlayerList", "Show player list", "Players can see who's online in the game's menu.", "False"),
	b("Server", "bIsUseBackupSaveData", "Game's own backups", "The game also keeps its own rolling world backups (Warden backs up separately).", "True"),
	i("Server", "PublicPort", "Game port", "The port players connect to. Change it only together with your firewall and router.", "8211", 1024, 65535),
	{Key: "AdminPassword", Group: "Server", Label: "Admin password", Help: "Warden signs in to the server with it, so change it in the file while Warden is stopped.", Type: "secret", Locked: true},
	{Key: "RESTAPIEnabled", Group: "Server", Label: "REST API", Help: "Warden needs it on.", Type: "bool", Default: "False", Locked: true},
	{Key: "RESTAPIPort", Group: "Server", Label: "REST API port", Help: "Warden reads it at startup.", Type: "int", Default: "8212", Locked: true},
	b("Server", "RCONEnabled", "RCON", "The older remote-console protocol. Warden doesn't need it; keep it off unless another tool does.", "False"),
}

// Lookup finds a catalog entry.
func Lookup(key string) (Option, bool) {
	for _, o := range Catalog {
		if o.Key == key {
			return o, true
		}
	}
	return Option{}, false
}
