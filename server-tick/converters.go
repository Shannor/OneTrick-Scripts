package main

import (
	"log/slog"
	"serverTick/bungie"
	"strconv"

	"github.com/rs/zerolog/log"
)

func TransformItemToDetails(
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
		result.Perks = generatePerks(item, perks)
	}

	// Generate Sockets
	if item.Sockets != nil && item.Sockets.Data != nil {
		result.Sockets = generateSockets(item, items)
	}

	// Generate Stats
	if item.Stats != nil && item.Stats.Data != nil {
		result.Stats = generateStats(item, stats)
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

func generatePerks(item *bungie.DestinyItem, perks map[string]PerkDefinition) []Perk {
	var results []Perk
	for _, p := range *item.Perks.Data.Perks {
		perk, ok := perks[strconv.Itoa(int(*p.PerkHash))]
		if !ok {
			log.Warn().Uint32("perkHash", *p.PerkHash).Msg("Perk not found in manifest")
			continue
		}
		if !perk.IsDisplayable {
			continue
		}
		results = append(results, Perk{
			Hash:        int64(*p.PerkHash),
			IconPath:    Of(setBaseBungieURL(p.IconPath)),
			Name:        perk.DisplayProperties.Name,
			Description: &perk.DisplayProperties.Description,
		})
	}
	return results
}

func generateSockets(item *bungie.DestinyItem, items map[string]ItemDefinition) *[]Socket {
	var sockets []Socket
	for _, s := range *item.Sockets.Data.Sockets {
		if s.PlugHash == nil {
			log.Warn().Msg("Socket has no plug hash")
			continue
		}
		socket, ok := items[strconv.Itoa(int(*s.PlugHash))]
		if !ok {
			log.Warn().Uint32("socketHash", *s.PlugHash).Msg("Socket not found in manifest")
			continue
		}

		hash := int(*s.PlugHash)
		sockets = append(sockets, Socket{
			IsEnabled:                 s.IsEnabled,
			IsVisible:                 s.IsVisible,
			PlugHash:                  hash,
			Name:                      socket.DisplayProperties.Name,
			Description:               socket.DisplayProperties.Description,
			ItemTypeDisplayName:       Of(socket.ItemTypeDisplayName),
			ItemTypeTieredDisplayName: Of(socket.ItemTypeAndTierDisplayName),
			Icon:                      Of(setBaseBungieURL(&socket.DisplayProperties.Icon)),
		})
	}
	return &sockets
}

func generateStats(item *bungie.DestinyItem, statDefinitions map[string]StatDefinition) Stats {
	stats := make(Stats)
	for key, s := range *item.Stats.Data.Stats {
		if s.StatHash == nil || s.Value == nil {
			slog.Warn("Missing stat hash or value for stat: ", key)
			continue
		}
		stat, ok := statDefinitions[strconv.Itoa(int(*s.StatHash))]
		if !ok {
			slog.Warn("Stat not found in manifest: ", strconv.Itoa(int(*s.StatHash)))
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

func TransformD2HistoricalStatValues(stats *map[string]bungie.HistoricalStatsValue) *map[string]UniqueStatValue {
	if stats == nil {
		return nil
	}

	result := make(map[string]UniqueStatValue)
	for key, value := range *stats {
		values := transformD2StatValue(&value)
		if values == nil {
			continue
		}
		result[key] = *values
	}

	return &result
}

func transformD2StatValue(item *bungie.HistoricalStatsValue) *UniqueStatValue {
	if item == nil {
		return nil
	}
	if item.Basic == nil {
		slog.Warn("Missing basic value for stat")
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
	return Of(int64(*item))
}

func TransformHistoricActivity(history *bungie.HistoricalStatsActivity, activityDefinition, directorDef ActivityDefinition, modeDefinition ActivityModeDefinition) *ActivityHistory {
	if history == nil {
		return nil
	}
	mode := ActivityModeTypeToString((*bungie.CurrentActivityModeType)(history.Mode))
	return &ActivityHistory{
		ActivityHash: *uintToInt64(history.DirectorActivityHash),
		InstanceID:   *history.InstanceId,
		IsPrivate:    history.IsPrivate,
		Mode:         &mode,
		ReferenceID:  *uintToInt64(history.ReferenceId),
		Location:     activityDefinition.DisplayProperties.Name,
		Description:  activityDefinition.DisplayProperties.Description,
		Activity:     directorDef.DisplayProperties.Name,
		ImageURL:     setBaseBungieURL(&activityDefinition.PgcrImage),
		ActivityIcon: setBaseBungieURL(&modeDefinition.DisplayProperties.Icon),
	}
}

func TransformPeriodGroups(period []bungie.StatsPeriodGroup, activities map[string]ActivityDefinition, directorDefinitions map[string]ActivityDefinition, modes map[string]ActivityModeDefinition) []ActivityHistory {
	if period == nil {
		return nil
	}
	var result []ActivityHistory
	for _, group := range period {
		r := TransformPeriodGroup(&group, activities, directorDefinitions, modes)
		if r == nil {
			log.Warn().Msg("period group returned nil")
			continue
		}
		result = append(result, *r)
	}
	return result
}

func TransformPeriodGroup(period *bungie.StatsPeriodGroup, activities map[string]ActivityDefinition, directorDefintions map[string]ActivityDefinition, modes map[string]ActivityModeDefinition) *ActivityHistory {
	if period == nil {
		return nil
	}

	definition, ok := activities[strconv.Itoa(int(*period.ActivityDetails.ReferenceId))]
	if !ok {
		log.Warn().Msgf("Activity locale not found in manifest: %d ", period.ActivityDetails.ReferenceId)
		return nil
	}
	directorDefinition, ok := directorDefintions[strconv.Itoa(int(*period.ActivityDetails.DirectorActivityHash))]
	if !ok {
		log.Warn().Msgf("Activity Directory not found in manifest: %d", period.ActivityDetails.DirectorActivityHash)
		return nil
	}
	activityMode := modes[strconv.Itoa(directorDefinition.DirectActivityModeHash)]
	mode := ActivityModeTypeToString((*bungie.CurrentActivityModeType)(period.ActivityDetails.Mode))
	return &ActivityHistory{
		ActivityHash: *uintToInt64(period.ActivityDetails.DirectorActivityHash),
		InstanceID:   *period.ActivityDetails.InstanceId,
		IsPrivate:    period.ActivityDetails.IsPrivate,
		Mode:         &mode,
		ReferenceID:  *uintToInt64(period.ActivityDetails.ReferenceId),
		Location:     definition.DisplayProperties.Name,
		Description:  definition.DisplayProperties.Description,
		Activity:     directorDefinition.DisplayProperties.Name,
		ImageURL:     setBaseBungieURL(&definition.PgcrImage),
		ActivityIcon: setBaseBungieURL(&activityMode.DisplayProperties.Icon),
		Period:       *period.Period,
	}
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
				Icon:        Of(setBaseBungieURL(&def.DisplayProperties.Icon)),
				Name:        def.DisplayProperties.Name,
			}
		}

		result[strconv.Itoa(int(*v.ReferenceId))] = r
	}
	return result
}

func ActivityModeTypeToString(modeType *bungie.CurrentActivityModeType) string {
	if modeType == nil {
		slog.Warn("Activity Mode type is nil")
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

func generateClassStats(statDefinitions map[string]StatDefinition, stats map[string]int32) map[string]ClassStat {
	if statDefinitions == nil {
		return nil
	}
	results := make(map[string]ClassStat)
	for key, value := range stats {
		info, ok := statDefinitions[key]
		if !ok {
			slog.Warn("Missing stat", "statKey", key)
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
