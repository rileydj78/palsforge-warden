package gamesettings

// Preset is a named set of values to start from. Picking one only fills the form: the admin
// reviews the changes and applies them like any other edit.
type Preset struct {
	Name   string            `json:"name"`
	About  string            `json:"about"`
	Values map[string]string `json:"values"`
}

// Presets offered on the Game settings page.
var Presets = []Preset{
	{"The game's defaults", "Everything back to Palworld's own values.", defaults()},
	{"Relaxed PvE", "Friendly co-op: faster progress, no PvP, keep your items on death.", map[string]string{
		"ExpRate": "2", "PalCaptureRate": "1.5", "EnemyDropItemRate": "2", "CollectionDropRate": "2", "PalEggDefaultHatchingTime": "1",
		"PlayerStomachDecreaceRate": "0.7", "DeathPenalty": "None", "bIsPvP": "False", "bEnablePlayerToPlayerDamage": "False",
		"bEnableFriendlyFire": "False", "BuildObjectDeteriorationDamageRate": "0"}},
	{"Boosted (2x)", "Everything twice as fast, otherwise the normal game.", map[string]string{
		"ExpRate": "2", "PalCaptureRate": "2", "EnemyDropItemRate": "2", "CollectionDropRate": "2", "WorkSpeedRate": "2", "PalEggDefaultHatchingTime": "36"}},
	{"Hardcore", "Slower progress, hungrier players, lose everything on death.", map[string]string{
		"ExpRate": "0.5", "PalCaptureRate": "0.7", "EnemyDropItemRate": "0.7", "CollectionDropRate": "0.7",
		"PlayerStomachDecreaceRate": "1.5", "PlayerDamageRateDefense": "1.5", "DeathPenalty": "All"}},
	{"Anarchy", "Full PvP, raiding, drop everything on death, huge rates, instant eggs, frequent supply drops.", map[string]string{
		"bIsPvP": "True", "bEnablePlayerToPlayerDamage": "True", "bEnableFriendlyFire": "True", "bCanPickupOtherGuildDeathPenaltyDrop": "True",
		"bEnableDefenseOtherGuildPlayer": "True", "DeathPenalty": "All", "bExistPlayerAfterLogout": "True", "bEnableInvaderEnemy": "True",
		"bShowPlayerList": "True", "ExpRate": "30", "EnemyDropItemRate": "10", "CollectionDropRate": "10", "CollectionObjectHpRate": "0.5",
		"CollectionObjectRespawnSpeedRate": "0.5", "PalCaptureRate": "5", "PalEggDefaultHatchingTime": "0", "WorkSpeedRate": "5",
		"PlayerStomachDecreaceRate": "0.5", "PlayerStaminaDecreaceRate": "0.5", "SupplyDropSpan": "30"}},
}

func defaults() map[string]string {
	out := map[string]string{}
	for _, o := range Catalog {
		if o.Default != "" && !o.Locked && o.Type != "string" && o.Type != "secret" {
			out[o.Key] = o.Default
		}
	}
	return out
}
