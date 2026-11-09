package main

import (
	"context"
	"serverTick/bungie"
	"strconv"
	"strings"

	clog "github.com/Shannor/canonical-logger"
)

func TransformItemToDetails(
	ctx context.Context,
	item *bungie.DestinyItem,
	items map[string]ItemDefinition,
	damages map[string]DamageType,
	perks map[string]PerkDefinition,
	stats map[string]StatDefinition,
	styleItem *ItemDefinition,
) *ItemProperties {
	if item == nil {
		return nil
	}
	result := ItemProperties{CharacterId: item.CharacterId}

	// Generate Base Info
	if item.Item != nil {
		result.BaseInfo = generateBaseInfo(item, items, damages, styleItem)
	}

	// Generate Perks
	if item.Perks != nil && item.Perks.Data != nil {
		result.Perks = generatePerks(ctx, item, perks)
	}

	// Generate Sockets
	if item.Sockets != nil && item.Sockets.Data != nil {
		result.Sockets = generateSockets(ctx, item, items)
	}

	// Generate Stats
	if item.Stats != nil && item.Stats.Data != nil {
		result.Stats = generateStats(ctx, item, stats)
	}

	return &result
}

func generateBaseInfo(item *bungie.DestinyItem, items map[string]ItemDefinition, damages map[string]DamageType, styleItem *ItemDefinition) BaseItemInfo {
	c := *item.Item.ItemComponent
	hash := strconv.Itoa(int(*c.ItemHash))
	name := items[hash].DisplayProperties.Name
	icon := items[hash].DisplayProperties.Icon
	it := items[hash]

	base := BaseItemInfo{
		BucketHash:                 int64(*c.BucketHash),
		InstanceId:                 *c.ItemInstanceId,
		ItemHash:                   int64(*c.ItemHash),
		Name:                       name,
		Icon:                       setBaseBungieURL(&icon),
		ItemTypeAndTierDisplayName: it.ItemTypeAndTierDisplayName,
		ItemTypeDisplayName:        it.ItemTypeDisplayName,
		TierTypeName:               it.Inventory.TierTypeName,
		TierType:                   it.Inventory.TierType,
	}

	if styleItem != nil {
		base.StyleBasicInfo = &BaseItemInfo{
			BucketHash:                 int64(*c.BucketHash),
			InstanceId:                 *c.ItemInstanceId,
			ItemHash:                   styleItem.Hash,
			Name:                       styleItem.DisplayProperties.Name,
			Icon:                       setBaseBungieURL(&styleItem.DisplayProperties.Icon),
			ItemTypeAndTierDisplayName: styleItem.ItemTypeAndTierDisplayName,
			ItemTypeDisplayName:        styleItem.ItemTypeDisplayName,
			TierTypeName:               styleItem.Inventory.TierTypeName,
			TierType:                   styleItem.Inventory.TierType,
		}
	}
	if item.Instance != nil {
		instance := *item.Instance.ItemInstanceComponent
		if instance.DamageTypeHash != nil {
			hash := strconv.Itoa(int(*instance.DamageTypeHash))
			def := damages[hash]
			dc := def.Color

			base.Damage = &DamageInfo{
				Color: Color{
					Alpha: dc.Alpha,
					Blue:  dc.Blue,
					Green: dc.Green,
					Red:   dc.Red,
				},
				DamageIcon:      def.DisplayProperties.Icon,
				DamageType:      def.DisplayProperties.Name,
				TransparentIcon: def.TransparentIconPath,
			}
		}
	}
	return base
}

func generatePerks(ctx context.Context, item *bungie.DestinyItem, perks map[string]PerkDefinition) []Perk {
	var results []Perk
	for _, p := range *item.Perks.Data.Perks {
		perk, ok := perks[strconv.Itoa(int(*p.PerkHash))]
		if !ok {
			clog.WithGroupField(ctx, "converterService.generatePerks", "warning", clog.Fields{
				"type":     "missing_perk",
				"perkHash": *p.PerkHash,
			})
			continue
		}
		if !perk.IsDisplayable {
			continue
		}
		results = append(results, Perk{
			Hash:        int64(*p.PerkHash),
			IconPath:    new(setBaseBungieURL(p.IconPath)),
			Name:        perk.DisplayProperties.Name,
			Description: &perk.DisplayProperties.Description,
		})
	}
	return results
}

func generateSockets(ctx context.Context, item *bungie.DestinyItem, items map[string]ItemDefinition) *[]Socket {
	var sockets []Socket
	for _, s := range *item.Sockets.Data.Sockets {
		if s.PlugHash == nil {
			clog.WithGroupField(ctx, "converterService.generateSockets", "warning", clog.Fields{
				"type": "missing_plug_hash",
			})
			continue
		}
		socket, ok := items[strconv.Itoa(int(*s.PlugHash))]
		if !ok {
			clog.WithGroupField(ctx, "converterService.generateSockets", "warning", clog.Fields{
				"type":       "missing_socket",
				"socketHash": *s.PlugHash,
			})
			continue
		}

		hash := int(*s.PlugHash)
		sockets = append(sockets, Socket{
			IsEnabled:                 s.IsEnabled,
			IsVisible:                 s.IsVisible,
			PlugHash:                  hash,
			Name:                      socket.DisplayProperties.Name,
			Description:               socket.DisplayProperties.Description,
			ItemTypeDisplayName:       new(socket.ItemTypeDisplayName),
			ItemTypeTieredDisplayName: new(socket.ItemTypeAndTierDisplayName),
			Icon:                      new(setBaseBungieURL(&socket.DisplayProperties.Icon)),
		})
	}
	return &sockets
}

func generateStats(ctx context.Context, item *bungie.DestinyItem, statDefinitions map[string]StatDefinition) Stats {
	stats := make(Stats)
	for key, s := range *item.Stats.Data.Stats {
		if s.StatHash == nil || s.Value == nil {
			clog.WithGroupField(ctx, "converterService.generateStats", "warning", clog.Fields{
				"type":    "missing_stat_hash_or_value",
				"statKey": key,
			})
			continue
		}
		stat, ok := statDefinitions[strconv.Itoa(int(*s.StatHash))]
		if !ok {
			clog.WithGroupField(ctx, "converterService.generateStats", "warning", clog.Fields{
				"type":     "missing_stat_definition",
				"statHash": *s.StatHash,
			})
			continue
		}
		value := int64(*s.Value)
		stats[key] = GunStat{
			Description: stat.DisplayProperties.Description,
			Hash:        stat.Hash,
			Name:        stat.DisplayProperties.Name,
			Value:       value,
		}
	}
	return stats
}

func TransformD2HistoricalStatValues(ctx context.Context, stats *map[string]bungie.HistoricalStatsValue) *map[string]UniqueStatValue {
	if stats == nil {
		return nil
	}

	result := make(map[string]UniqueStatValue)
	for key, value := range *stats {
		values := transformD2StatValue(ctx, &value)
		if values == nil {
			continue
		}
		result[key] = *values
	}

	return &result
}

func transformD2StatValue(ctx context.Context, item *bungie.HistoricalStatsValue) *UniqueStatValue {
	if item == nil {
		return nil
	}
	if item.Basic == nil {
		clog.WithGroupField(ctx, "converterService.transformD2StatValue", "warning", clog.Fields{
			"type": "missing_basic_stat_value",
		})
		return nil
	}
	result := &UniqueStatValue{
		ActivityID: item.ActivityId,
	}
	if item.Basic != nil {
		result.Basic = StatsValuePair{
			DisplayValue: item.Basic.DisplayValue,
			Value:        item.Basic.Value,
		}
	}
	if item.Pga != nil {
		result.Pga = &StatsValuePair{
			DisplayValue: item.Pga.DisplayValue,
			Value:        item.Pga.Value,
		}
	}
	if item.Weighted != nil {
		result.Weighted = &StatsValuePair{
			DisplayValue: item.Weighted.DisplayValue,
			Value:        item.Weighted.Value,
		}
	}
	return result
}

func uintToInt64[T ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](item *T) *int64 {
	if item == nil {
		return nil
	}
	return new(int64(*item))
}

func TransformHistoricActivity(ctx context.Context, history *bungie.HistoricalStatsActivity, activityDefinition, directorDef ActivityDefinition, modeDefinition ActivityModeDefinition) *ActivityHistory {
	if history == nil {
		return nil
	}
	mode := ActivityModeTypeToString(ctx, (*bungie.CurrentActivityModeType)(history.Mode))
	estimatedGameMode := EstimateGameMode(&mode, directorDef.DisplayProperties.Name)
	return &ActivityHistory{
		ActivityHash:      *uintToInt64(history.DirectorActivityHash),
		InstanceID:        *history.InstanceId,
		IsPrivate:         history.IsPrivate,
		Mode:              &mode,
		EstimatedGameMode: estimatedGameMode,
		ReferenceID:       *uintToInt64(history.ReferenceId),
		Location:          activityDefinition.DisplayProperties.Name,
		Description:       activityDefinition.DisplayProperties.Description,
		Activity:          directorDef.DisplayProperties.Name,
		ImageURL:          setBaseBungieURL(&activityDefinition.PgcrImage),
		ActivityIcon:      setBaseBungieURL(&modeDefinition.DisplayProperties.Icon),
	}
}

func TransformPeriodGroups(ctx context.Context, period []bungie.StatsPeriodGroup, activities map[string]ActivityDefinition, directorDefinitions map[string]ActivityDefinition, modes map[string]ActivityModeDefinition) []ActivityHistory {
	if period == nil {
		return nil
	}
	var result []ActivityHistory
	for _, group := range period {
		r := TransformPeriodGroup(ctx, &group, activities, directorDefinitions, modes)
		if r == nil {
			continue
		}
		result = append(result, *r)
	}
	return result
}

func TransformPeriodGroup(ctx context.Context, period *bungie.StatsPeriodGroup, activities map[string]ActivityDefinition, directorDefintions map[string]ActivityDefinition, modes map[string]ActivityModeDefinition) *ActivityHistory {
	if period == nil {
		return nil
	}

	definition, ok := activities[strconv.Itoa(int(*period.ActivityDetails.ReferenceId))]
	if !ok {
		clog.WithGroupField(ctx, "converterService.transformPeriodGroup", "warning", clog.Fields{
			"type":        "missing_activity_definition",
			"referenceId": *period.ActivityDetails.ReferenceId,
		})
		return nil
	}
	directorDefinition, ok := directorDefintions[strconv.Itoa(int(*period.ActivityDetails.DirectorActivityHash))]
	if !ok {
		clog.WithGroupField(ctx, "converterService.transformPeriodGroup", "warning", clog.Fields{
			"type":                 "missing_director_activity_definition",
			"directorActivityHash": *period.ActivityDetails.DirectorActivityHash,
		})
		return nil
	}

	// Exclude generic Private Matches unless the activity is Iron Banner or Tribute
	if period.ActivityDetails != nil && period.ActivityDetails.IsPrivate != nil && *period.ActivityDetails.IsPrivate {
		dirNameLower := strings.ToLower(directorDefinition.DisplayProperties.Name)
		defNameLower := strings.ToLower(definition.DisplayProperties.Name)
		isTributeOrIB := strings.Contains(dirNameLower, "tribute") ||
			strings.Contains(dirNameLower, "iron banner") ||
			strings.Contains(defNameLower, "tribute") ||
			strings.Contains(defNameLower, "iron banner")
		if !isTributeOrIB {
			return nil
		}
	}

	activityMode, ok := modes[strconv.Itoa(directorDefinition.DirectActivityModeHash)]
	if !ok && period.ActivityDetails.Mode != nil {
		activityMode = modes[strconv.Itoa(int(*period.ActivityDetails.Mode))]
	}
	mode := ActivityModeTypeToString(ctx, (*bungie.CurrentActivityModeType)(period.ActivityDetails.Mode))
	estimatedGameMode := EstimateGameMode(&mode, directorDefinition.DisplayProperties.Name)
	activityIcon := ""
	if activityMode.DisplayProperties.Icon != "" {
		activityIcon = setBaseBungieURL(&activityMode.DisplayProperties.Icon)
	}
	return &ActivityHistory{
		ActivityHash:      *uintToInt64(period.ActivityDetails.DirectorActivityHash),
		InstanceID:        *period.ActivityDetails.InstanceId,
		IsPrivate:         period.ActivityDetails.IsPrivate,
		Mode:              &mode,
		EstimatedGameMode: estimatedGameMode,
		ReferenceID:       *uintToInt64(period.ActivityDetails.ReferenceId),
		Location:          definition.DisplayProperties.Name,
		Description:       definition.DisplayProperties.Description,
		Activity:          directorDefinition.DisplayProperties.Name,
		ImageURL:          setBaseBungieURL(&definition.PgcrImage),
		ActivityIcon:      activityIcon,
		Period:            *period.Period,
	}
}

// EstimateGameMode classifies an activity into one of the four high-level PvP GameMode buckets
// ("ironBanner", "trials", "competitive", "quickplay") using both the Bungie mode string and
// the director playlist activity name without mutating the original fields.
func EstimateGameMode(mode *string, activity string) *GameMode {
	modeLower := ""
	if mode != nil {
		modeLower = strings.ToLower(strings.TrimSpace(*mode))
	}
	actLower := strings.ToLower(strings.TrimSpace(activity))

	// 1. Iron Banner (e.g. mode="Iron Banner Control" with activity="Control: Matchmade", or activity="Iron Banner: Tribute" / "Eruption: Matchmade")
	if strings.Contains(modeLower, "iron banner") ||
		strings.Contains(actLower, "iron banner") ||
		strings.Contains(actLower, "eruption") ||
		strings.Contains(actLower, "fortress") ||
		strings.Contains(actLower, "tribute") {
		gm := GameModeIronBanner
		return &gm
	}

	// 2. Trials of Osiris
	if strings.Contains(modeLower, "trials") || strings.Contains(actLower, "trials") {
		gm := GameModeTrials
		return &gm
	}

	// 3. Competitive (e.g. activity="Competitive: Matchmade" or "Competitive" with mode="Clash Quickplay" / "Zone Control", or mode="Survival" / "Control Competitive" / "Clash Competitive")
	if strings.Contains(actLower, "competitive") ||
		strings.Contains(modeLower, "competitive") ||
		modeLower == "survival" {
		gm := GameModeCompetitive
		return &gm
	}

	// 4. Quickplay (standard Crucible modes not matched by Iron Banner, Trials, or Competitive)
	if modeLower != "" && modeLower != "unknown" &&
		modeLower != "pve" && modeLower != "raid" && modeLower != "nightfall" && modeLower != "gambit" &&
		!strings.Contains(modeLower, "private matches") {
		gm := GameModeQuickplay
		return &gm
	}
	if strings.Contains(actLower, "control") ||
		strings.Contains(actLower, "clash") ||
		strings.Contains(actLower, "quickplay") ||
		strings.Contains(actLower, "rumble") ||
		strings.Contains(actLower, "supremacy") ||
		strings.Contains(actLower, "mayhem") ||
		strings.Contains(actLower, "relic") ||
		strings.Contains(actLower, "checkmate") ||
		strings.Contains(actLower, "momentum") ||
		strings.Contains(actLower, " rift") ||
		strings.HasPrefix(actLower, "rift") ||
		strings.Contains(actLower, "doubles") ||
		strings.Contains(actLower, "elimination") {
		gm := GameModeQuickplay
		return &gm
	}

	return nil
}

func ToPlayerStats(values *map[string]bungie.HistoricalStatsValue) *PlayerStats {
	if values == nil {
		return nil
	}
	personalValues := &PlayerStats{}
	for key, value := range *values {
		switch key {
		case "kills":
			personalValues.Kills = (*StatsValuePair)(value.Basic)
		case "assists":
			personalValues.Assists = (*StatsValuePair)(value.Basic)
		case "deaths":
			personalValues.Deaths = (*StatsValuePair)(value.Basic)
		case "killsDeathsRatio":
			personalValues.Kd = (*StatsValuePair)(value.Basic)
		case "killsDeathsAssists":
			personalValues.Kda = (*StatsValuePair)(value.Basic)
		case "standing":
			personalValues.Standing = (*StatsValuePair)(value.Basic)
		case "fireteamId":
			personalValues.FireTeamID = (*StatsValuePair)(value.Basic)
		case "timePlayedSeconds":
			personalValues.TimePlayed = (*StatsValuePair)(value.Basic)
		}
	}
	return personalValues
}

func CarnageEntryToInstancePerformance(entry *bungie.PostGameCarnageReportEntry, items map[string]ItemDefinition) *InstancePerformance {
	if entry == nil {
		return nil
	}
	result := &InstancePerformance{}

	result.Extra = BungieStatValueToUniqueStatValue(entry.Extended.Values)
	result.PlayerStats = *ToPlayerStats(entry.Values)
	result.Weapons = WeaponsToInstanceWeapons(entry.Extended.Weapons, items)
	return result
}

func BungieStatValueToUniqueStatValue(values *map[string]bungie.HistoricalStatsValue) *map[string]UniqueStatValue {
	if values == nil {
		return nil
	}
	result := make(map[string]UniqueStatValue)
	for key, value := range *values {
		result[key] = UniqueStatValue{
			ActivityID: value.ActivityId,
			Basic: StatsValuePair{
				DisplayValue: value.Basic.DisplayValue,
				Value:        value.Basic.Value,
			},
			Name: value.StatId,
		}
	}
	return &result
}

func WeaponsToInstanceWeapons(values *[]bungie.HistoricalWeaponStats, items map[string]ItemDefinition) map[string]WeaponInstanceMetrics {
	if values == nil {
		return nil
	}
	result := make(map[string]WeaponInstanceMetrics)
	for _, v := range *values {
		if v.ReferenceId == nil {
			continue
		}
		ref := int64(*v.ReferenceId)
		if ref == 0 {
			continue
		}
		r := WeaponInstanceMetrics{
			ReferenceID: &ref,
			Stats:       BungieStatValueToUniqueStatValue(v.Values),
		}
		def, ok := items[strconv.Itoa(int(*v.ReferenceId))]
		if ok {
			r.Display = &Display{
				Description: def.ItemTypeAndTierDisplayName,
				HasIcon:     def.DisplayProperties.HasIcon,
				Icon:        new(setBaseBungieURL(&def.DisplayProperties.Icon)),
				Name:        def.DisplayProperties.Name,
			}
		}

		result[strconv.Itoa(int(*v.ReferenceId))] = r
	}
	return result
}

func ActivityModeTypeToString(ctx context.Context, modeType *bungie.CurrentActivityModeType) string {
	if modeType == nil {
		clog.WithGroupField(ctx, "converterService.activityModeTypeToString", "warning", clog.Fields{
			"type": "nil_activity_mode",
		})
		return "Unknown"
	}
	switch *modeType {
	case bungie.CurrentActivityModeTypeControl:
		return "Control"
	case bungie.CurrentActivityModeTypeIronBannerZoneControl:
		return "Iron Banner Zone Control"
	case bungie.CurrentActivityModeTypeIronBannerControl:
		return "Iron Banner Control"
	case bungie.CurrentActivityModeTypeZoneControl:
		return "Zone Control"
	case bungie.CurrentActivityModeTypeControlCompetitive:
		return "Control Competitive"
	case bungie.CurrentActivityModeTypeControlQuickplay:
		return "Control Quickplay"
	case bungie.CurrentActivityModeTypePrivateMatchesControl:
		return "Private Matches Control"
	case bungie.CurrentActivityModeTypeAllDoubles:
		return "Doubles"
	case bungie.CurrentActivityModeTypeAllPvE:
		return "PvE"
	case bungie.CurrentActivityModeTypeAllPvP:
		return "PvP"
	case bungie.CurrentActivityModeTypeClash:
		return "Clash"
	case bungie.CurrentActivityModeTypeClashQuickplay:
		return "Clash Quickplay"
	case bungie.CurrentActivityModeTypeClashCompetitive:
		return "Clash Competitive"
	case bungie.CurrentActivityModeTypeIronBannerRift:
		return "Iron Banner Rift"
	case bungie.CurrentActivityModeTypeRift:
		return "Rift"
	case bungie.CurrentActivityModeTypeIronBannerClash:
		return "Iron Banner Clash"
	case bungie.CurrentActivityModeTypeIronBannerSupremacy:
		return "Iron Banner Supremacy"
	case bungie.CurrentActivityModeTypePrivateMatchesSurvival:
		return "Private Matches Survival"
	case bungie.CurrentActivityModeTypeTrialsSurvival:
		return "Trials Survival"
	case bungie.CurrentActivityModeTypeTrialsCountdown:
		return "Trials Countdown"
	case bungie.CurrentActivityModeTypeRaid:
		return "Raid"
	case bungie.CurrentActivityModeTypeNightfall:
		return "Nightfall"
	case bungie.CurrentActivityModeTypeGambit:
		return "Gambit"
	case bungie.CurrentActivityModeTypeIronBanner:
		return "Iron Banner"
	case bungie.CurrentActivityModeTypeTrialsOfOsiris:
		return "Trials of Osiris"
	case bungie.CurrentActivityModeTypeSurvival:
		return "Survival"
	default:
		return "Unknown"
	}
}

func generateClassStats(ctx context.Context, statDefinitions map[string]StatDefinition, stats map[string]int32) map[string]ClassStat {
	if statDefinitions == nil {
		return nil
	}
	results := make(map[string]ClassStat)
	for key, value := range stats {
		info, ok := statDefinitions[key]
		if !ok {
			clog.WithGroupField(ctx, "converterService.generateClassStats", "warning", clog.Fields{
				"type":    "missing_stat_definition",
				"statKey": key,
			})
			continue
		}
		i := ClassStat{
			Name:            info.DisplayProperties.Name,
			Icon:            setBaseBungieURL(&info.DisplayProperties.Icon),
			HasIcon:         info.DisplayProperties.HasIcon,
			Description:     info.DisplayProperties.Description,
			StatCategory:    info.StatCategory,
			AggregationType: info.AggregationType,
			Value:           value,
		}
		results[key] = i
	}
	return results
}
