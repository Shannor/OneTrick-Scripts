package main

import (
	"errors"
)

var ErrDestinyServerDown = errors.New("destiny server is down")

type Manifest struct {
	ArtDyeChannelDefinition                  map[string]any                       `json:"DestinyArtDyeChannelDefinition"`
	ArtDyeReferenceDefinition                map[string]any                       `json:"DestinyArtDyeReferenceDefinition"`
	PlaceDefinition                          map[string]PlaceDefinition           `json:"DestinyPlaceDefinition"`
	ActivityDefinition                       map[string]ActivityDefinition        `json:"DestinyActivityDefinition"`
	ActivityTypeDefinition                   map[string]any                       `json:"DestinyActivityTypeDefinition"`
	ClassDefinition                          map[string]ClassDefinition           `json:"DestinyClassDefinition"`
	GenderDefinition                         map[string]any                       `json:"DestinyGenderDefinition"`
	InventoryBucketDefinition                map[string]InventoryBucketDefinition `json:"DestinyInventoryBucketDefinition"`
	RaceDefinition                           map[string]RaceDefinition            `json:"DestinyRaceDefinition"`
	UnlockDefinition                         map[string]any                       `json:"DestinyUnlockDefinition"`
	StatGroupDefinition                      map[string]any                       `json:"DestinyStatGroupDefinition"`
	ProgressionMappingDefinition             map[string]any                       `json:"DestinyProgressionMappingDefinition"`
	FactionDefinition                        map[string]any                       `json:"DestinyFactionDefinition"`
	VendorGroupDefinition                    map[string]any                       `json:"DestinyVendorGroupDefinition"`
	RewardSourceDefinition                   map[string]any                       `json:"DestinyRewardSourceDefinition"`
	UnlockValueDefinition                    map[string]any                       `json:"DestinyUnlockValueDefinition"`
	RewardMappingDefinition                  map[string]any                       `json:"DestinyRewardMappingDefinition"`
	RewardSheetDefinition                    map[string]any                       `json:"DestinyRewardSheetDefinition"`
	ItemCategoryDefinition                   map[string]ItemCategory              `json:"DestinyItemCategoryDefinition"`
	DamageTypeDefinition                     map[string]DamageType                `json:"DestinyDamageTypeDefinition"`
	ActivityModeDefinition                   map[string]ActivityModeDefinition    `json:"DestinyActivityModeDefinition"`
	MedalTierDefinition                      map[string]any                       `json:"DestinyMedalTierDefinition"`
	AchievementDefinition                    map[string]any                       `json:"DestinyAchievementDefinition"`
	ActivityGraphDefinition                  map[string]any                       `json:"DestinyActivityGraphDefinition"`
	ActivityInteractableDefinition           map[string]any                       `json:"DestinyActivityInteractableDefinition"`
	BondDefinition                           map[string]any                       `json:"DestinyBondDefinition"`
	CharacterCustomizationCategoryDefinition map[string]any                       `json:"DestinyCharacterCustomizationCategoryDefinition"`
	CharacterCustomizationOptionDefinition   map[string]any                       `json:"DestinyCharacterCustomizationOptionDefinition"`
	CollectibleDefinition                    map[string]any                       `json:"DestinyCollectibleDefinition"`
	DestinationDefinition                    map[string]any                       `json:"DestinyDestinationDefinition"`
	EntitlementOfferDefinition               map[string]any                       `json:"DestinyEntitlementOfferDefinition"`
	EquipmentSlotDefinition                  map[string]any                       `json:"DestinyEquipmentSlotDefinition"`
	EventCardDefinition                      map[string]any                       `json:"DestinyEventCardDefinition"`
	FireteamFinderActivityGraphDefinition    map[string]any                       `json:"DestinyFireteamFinderActivityGraphDefinition"`
	FireteamFinderActivitySetDefinition      map[string]any                       `json:"DestinyFireteamFinderActivitySetDefinition"`
	FireteamFinderLabelDefinition            map[string]any                       `json:"DestinyFireteamFinderLabelDefinition"`
	FireteamFinderLabelGroupDefinition       map[string]any                       `json:"DestinyFireteamFinderLabelGroupDefinition"`
	FireteamFinderOptionDefinition           map[string]any                       `json:"DestinyFireteamFinderOptionDefinition"`
	FireteamFinderOptionGroupDefinition      map[string]any                       `json:"DestinyFireteamFinderOptionGroupDefinition"`
	StatDefinition                           map[string]StatDefinition            `json:"DestinyStatDefinition"`
	InventoryItemDefinition                  map[string]ItemDefinition            `json:"DestinyInventoryItemDefinition"`
	InventoryItemLiteDefinition              map[string]any                       `json:"DestinyInventoryItemLiteDefinition"`
	ItemTierTypeDefinition                   map[string]any                       `json:"DestinyItemTierTypeDefinition"`
	LoadoutColorDefinition                   map[string]any                       `json:"DestinyLoadoutColorDefinition"`
	LoadoutIconDefinition                    map[string]any                       `json:"DestinyLoadoutIconDefinition"`
	LoadoutNameDefinition                    map[string]any                       `json:"DestinyLoadoutNameDefinition"`
	LocationDefinition                       map[string]any                       `json:"DestinyLocationDefinition"`
	LoreDefinition                           map[string]any                       `json:"DestinyLoreDefinition"`
	MaterialRequirementSetDefinition         map[string]any                       `json:"DestinyMaterialRequirementSetDefinition"`
	MetricDefinition                         map[string]any                       `json:"DestinyMetricDefinition"`
	ObjectiveDefinition                      map[string]any                       `json:"DestinyObjectiveDefinition"`
	SandboxPerkDefinition                    map[string]PerkDefinition            `json:"DestinySandboxPerkDefinition"`
	PlatformBucketMappingDefinition          map[string]any                       `json:"DestinyPlatformBucketMappingDefinition"`
	PlugSetDefinition                        map[string]any                       `json:"DestinyPlugSetDefinition"`
	PowerCapDefinition                       map[string]any                       `json:"DestinyPowerCapDefinition"`
	PresentationNodeDefinition               map[string]any                       `json:"DestinyPresentationNodeDefinition"`
	ProgressionDefinition                    map[string]any                       `json:"DestinyProgressionDefinition"`
	ProgressionLevelRequirementDefinition    map[string]any                       `json:"DestinyProgressionLevelRequirementDefinition"`
	RecordDefinition                         map[string]RecordDefinition          `json:"DestinyRecordDefinition"`
	RewardAdjusterPointerDefinition          map[string]any                       `json:"DestinyRewardAdjusterPointerDefinition"`
	RewardAdjusterProgressionMapDefinition   map[string]any                       `json:"DestinyRewardAdjusterProgressionMapDefinition"`
	RewardItemListDefinition                 map[string]any                       `json:"DestinyRewardItemListDefinition"`
	SackRewardItemListDefinition             map[string]any                       `json:"DestinySackRewardItemListDefinition"`
	SandboxPatternDefinition                 map[string]any                       `json:"DestinySandboxPatternDefinition"`
	SeasonDefinition                         map[string]any                       `json:"DestinySeasonDefinition"`
	SeasonPassDefinition                     map[string]any                       `json:"DestinySeasonPassDefinition"`
	SocialCommendationDefinition             map[string]any                       `json:"DestinySocialCommendationDefinition"`
	SocketCategoryDefinition                 map[string]any                       `json:"DestinySocketCategoryDefinition"`
	SocketTypeDefinition                     map[string]any                       `json:"DestinySocketTypeDefinition"`
	TraitDefinition                          map[string]any                       `json:"DestinyTraitDefinition"`
	UnlockCountMappingDefinition             map[string]any                       `json:"DestinyUnlockCountMappingDefinition"`
	UnlockEventDefinition                    map[string]any                       `json:"DestinyUnlockEventDefinition"`
	UnlockExpressionMappingDefinition        map[string]any                       `json:"DestinyUnlockExpressionMappingDefinition"`
	VendorDefinition                         map[string]any                       `json:"DestinyVendorDefinition"`
	MilestoneDefinition                      map[string]any                       `json:"DestinyMilestoneDefinition"`
	ActivityModifierDefinition               map[string]any                       `json:"DestinyActivityModifierDefinition"`
	ReportReasonCategoryDefinition           map[string]any                       `json:"DestinyReportReasonCategoryDefinition"`
	ArtifactDefinition                       map[string]any                       `json:"DestinyArtifactDefinition"`
	BreakerTypeDefinition                    map[string]any                       `json:"DestinyBreakerTypeDefinition"`
	ChecklistDefinition                      map[string]any                       `json:"DestinyChecklistDefinition"`
	EnergyTypeDefinition                     map[string]any                       `json:"DestinyEnergyTypeDefinition"`
	SocialCommendationNodeDefinition         map[string]any                       `json:"DestinySocialCommendationNodeDefinition"`
	GuardianRankDefinition                   map[string]any                       `json:"DestinyGuardianRankDefinition"`
	GuardianRankConstantsDefinition          map[string]any                       `json:"DestinyGuardianRankConstantsDefinition"`
	LoadoutConstantsDefinition               map[string]any                       `json:"DestinyLoadoutConstantsDefinition"`
	FireteamFinderConstantsDefinition        map[string]any                       `json:"DestinyFireteamFinderConstantsDefinition"`
	GlobalConstantsDefinition                map[string]any                       `json:"DestinyGlobalConstantsDefinition"`
}

// PlaceDefinition Information around all places a player could actually go in Destiny 2
type PlaceDefinition struct {
	DisplayProperties PlaceDisplayProperties `json:"displayProperties" firestore:"displayProperties"`
	Hash              int64                  `json:"hash" firestore:"hash"`
	Index             int                    `json:"index" firestore:"index"`
	Redacted          bool                   `json:"redacted" firestore:"redacted"`
	Blacklisted       bool                   `json:"blacklisted" firestore:"blacklisted"`
}

type PlaceDisplayProperties struct {
	Description string `json:"description" firestore:"description"`
	Name        string `json:"name" firestore:"name"`
	Icon        string `json:"icon" firestore:"icon"`
	HasIcon     bool   `json:"hasIcon" firestore:"hasIcon"`
}
type ClassDefinition struct {
	ClassType                      int                    `json:"classType" firestore:"classType"`
	DisplayProperties              ClassDisplayProperties `json:"displayProperties" firestore:"displayProperties"`
	GenderedClassNames             map[string]string      `json:"genderedClassNames" firestore:"genderedClassNames"`
	GenderedClassNamesByGenderHash map[string]string      `json:"genderedClassNamesByGenderHash" firestore:"genderedClassNamesByGenderHash"`
	Hash                           int64                  `json:"hash" firestore:"hash"`
	Index                          int                    `json:"index" firestore:"index"`
	Redacted                       bool                   `json:"redacted" firestore:"redacted"`
	Blacklisted                    bool                   `json:"blacklisted" firestore:"blacklisted"`
}

type ClassDisplayProperties struct {
	Name    string `json:"name"`
	HasIcon bool   `json:"hasIcon"`
}
type InventoryBucketDefinition struct {
	DisplayProperties      InventoryDisplayProperties `json:"displayProperties" firestore:"displayProperties"`
	Scope                  int                        `json:"scope" firestore:"scope"`
	Category               int                        `json:"category" firestore:"category"`
	BucketOrder            int                        `json:"bucketOrder" firestore:"bucketOrder"`
	ItemCount              int                        `json:"itemCount" firestore:"itemCount"`
	Location               int                        `json:"location" firestore:"location"`
	HasTransferDestination bool                       `json:"hasTransferDestination" firestore:"hasTransferDestination"`
	Enabled                bool                       `json:"enabled" firestore:"enabled"`
	FIFO                   bool                       `json:"fifo" firestore:"fifo"`
	Hash                   int64                      `json:"hash" firestore:"hash"`
	Index                  int                        `json:"index" firestore:"index"`
	Redacted               bool                       `json:"redacted" firestore:"redacted"`
	Blacklisted            bool                       `json:"blacklisted" firestore:"blacklisted"`
}

type InventoryDisplayProperties struct {
	Description string `json:"description,omitempty" firestore:"description"`
	Name        string `json:"name" firestore:"name"`
	HasIcon     bool   `json:"hasIcon" firestore:"hasIcon"`
}

type ItemCategory struct {
	Hash                    int64               `json:"hash" firestore:"hash"`
	Index                   int                 `json:"index" firestore:"index"`
	Visible                 bool                `json:"visible" firestore:"visible"`
	Deprecated              bool                `json:"deprecated" firestore:"deprecated"`
	ShortTitle              string              `json:"shortTitle" firestore:"shortTitle"`
	DisplayProperties       ItemCategoryDisplay `json:"displayProperties" firestore:"displayProperties"`
	GroupCategoryOnly       bool                `json:"groupCategoryOnly" firestore:"groupCategoryOnly"`
	ParentCategoryHashes    []int64             `json:"parentCategoryHashes" firestore:"parentCategoryHashes"`
	GroupedCategoryHashes   []int64             `json:"groupedCategoryHashes" firestore:"groupedCategoryHashes"`
	ItemTypeRegex           string              `json:"itemTypeRegex" firestore:"itemTypeRegex"`
	GrantDestinyItemType    int64               `json:"grantDestinyItemType" firestore:"grantDestinyItemType"`
	GrantDestinySubType     int64               `json:"grantDestinySubType" firestore:"grantDestinySubType"`
	GrantDestinyClass       int64               `json:"grantDestinyClass" firestore:"grantDestinyClass"`
	GrantDestinyBreakerType int64               `json:"grantDestinyBreakerType" firestore:"grantDestinyBreakerType"`
	OriginBucketIdentifier  string              `json:"originBucketIdentifier" firestore:"originBucketIdentifier"`
	IsPlug                  bool                `json:"isPlug" firestore:"isPlug"`
	Redacted                bool                `json:"redacted" firestore:"redacted"`
	Blacklisted             bool                `json:"blacklisted" firestore:"blacklisted"`
}

type ItemCategoryDisplay struct {
	Name        string `json:"name" firestore:"name"`
	Description string `json:"description" firestore:"description"`
	HasIcon     bool   `json:"hasIcon" firestore:"hasIcon"`
}

type ItemDefinition struct {
	Hash                       int64                 `json:"hash" firestore:"hash"`
	Index                      int                   `json:"index" firestore:"index"`
	DisplayProperties          ItemDisplayProperties `json:"displayProperties" firestore:"displayProperties"`
	Inventory                  Inventory             `json:"inventory" firestore:"inventory"`
	Stats                      ItemStats             `json:"stats" firestore:"stats"`
	EquippingBlock             EquippingBlock        `json:"equippingBlock" firestore:"equippingBlock"`
	TranslationBlock           TranslationBlock      `json:"translationBlock" firestore:"translationBlock"`
	Quality                    Quality               `json:"quality" firestore:"quality"`
	InvestmentStats            []InvestmentStat      `json:"investmentStats" firestore:"investmentStats"`
	Perks                      []ItemPerk            `json:"perks" firestore:"perks"`
	AllowActions               bool                  `json:"allowActions" firestore:"allowActions"`
	ItemTypeDisplayName        string                `json:"itemTypeDisplayName" firestore:"itemTypeDisplayName"`
	NonTransferrable           bool                  `json:"nonTransferrable" firestore:"nonTransferrable"`
	ItemTypeAndTierDisplayName string                `json:"itemTypeAndTierDisplayName" firestore:"itemTypeAndTierDisplayName"`
	ItemCategoryHashes         []int64               `json:"itemCategoryHashes" firestore:"itemCategoryHashes"`
	SpecialItemType            int                   `json:"specialItemType" firestore:"specialItemType"`
	ItemType                   int                   `json:"itemType" firestore:"itemType"`
	ItemSubType                int                   `json:"itemSubType" firestore:"itemSubType"`
	ClassType                  int                   `json:"classType" firestore:"classType"`
	BreakerType                int                   `json:"breakerType" firestore:"breakerType"`
	Equippable                 bool                  `json:"equippable" firestore:"equippable"`
	DefaultDamageType          int                   `json:"defaultDamageType" firestore:"defaultDamageType"`
	IsWrapper                  bool                  `json:"isWrapper" firestore:"isWrapper"`
	TraitIds                   []string              `json:"traitIds" firestore:"traitIds"`
	TraitHashes                []int64               `json:"traitHashes" firestore:"traitHashes"`
	Redacted                   bool                  `json:"redacted" firestore:"redacted"`
	Blacklisted                bool                  `json:"blacklisted" firestore:"blacklisted"`
}

type ItemDisplayProperties struct {
	Name        string `json:"name" firestore:"name"`
	Description string `json:"description" firestore:"description"`
	Icon        string `json:"icon" firestore:"icon"`
	HasIcon     bool   `json:"hasIcon" firestore:"hasIcon"`
}

type Inventory struct {
	MaxStackSize             int    `json:"maxStackSize" firestore:"maxStackSize"`
	BucketTypeHash           int64  `json:"bucketTypeHash" firestore:"bucketTypeHash"`
	TierTypeHash             int64  `json:"tierTypeHash" firestore:"tierTypeHash"`
	IsInstanceItem           bool   `json:"isInstanceItem" firestore:"isInstanceItem"`
	NonTransferrableOriginal bool   `json:"nonTransferrableOriginal" firestore:"nonTransferrableOriginal"`
	TierTypeName             string `json:"tierTypeName" firestore:"tierTypeName"`
	TierType                 int    `json:"tierType" firestore:"tierType"`
}

type ItemStats struct {
	DisablePrimaryStatDisplay bool                `json:"disablePrimaryStatDisplay" firestore:"disablePrimaryStatDisplay"`
	StatGroupHash             int64               `json:"statGroupHash" firestore:"statGroupHash"`
	Stats                     map[string]ItemStat `json:"stats" firestore:"stats"`
	HasDisplayableStats       bool                `json:"hasDisplayableStats" firestore:"hasDisplayableStats"`
	PrimaryBaseStatHash       int64               `json:"primaryBaseStatHash" firestore:"primaryBaseStatHash"`
}
type ItemPerk struct {
	PerkHash                 int64  `json:"perkHash" firestore:"perkHash"`
	PerkVisibility           int    `json:"perkVisibility" firestore:"perkVisibility"`
	RequirementDisplayString string `json:"requirementDisplayString" firestore:"requirementDisplayString"`
}
type ItemStat struct {
	StatHash       int64 `json:"statHash" firestore:"statHash"`
	Value          int   `json:"value" firestore:"value"`
	Minimum        int   `json:"minimum" firestore:"minimum"`
	Maximum        int   `json:"maximum" firestore:"maximum"`
	DisplayMaximum int   `json:"displayMaximum" firestore:"displayMaximum"`
}

type EquippingBlock struct {
	UniqueLabelHash       int64 `json:"uniqueLabelHash" firestore:"uniqueLabelHash"`
	EquipmentSlotTypeHash int64 `json:"equipmentSlotTypeHash" firestore:"equipmentSlotTypeHash"`
}

type TranslationBlock struct {
}

type Quality struct {
}

type InvestmentStat struct {
	StatTypeHash          int64 `json:"statTypeHash" firestore:"statTypeHash"`
	Value                 int   `json:"value" firestore:"value"`
	IsConditionallyActive bool  `json:"isConditionallyActive" firestore:"isConditionallyActive"`
}
type ActivityDefinition struct {
	ActivityLightLevel        int                       `json:"activityLightLevel" firestore:"activityLightLevel"`
	ActivityLocationMappings  []any                     `json:"activityLocationMappings" firestore:"activityLocationMappings"`
	ActivityModeHashes        []int                     `json:"activityModeHashes" firestore:"activityModeHashes"`
	ActivityModeTypes         []int                     `json:"activityModeTypes" firestore:"activityModeTypes"`
	ActivityTypeHash          int                       `json:"activityTypeHash" firestore:"activityTypeHash"`
	Blacklisted               bool                      `json:"blacklisted" firestore:"blacklisted"`
	Challenges                []any                     `json:"challenges" firestore:"challenges"`
	CompletionUnlockHash      int                       `json:"completionUnlockHash" firestore:"completionUnlockHash"`
	DestinationHash           int                       `json:"destinationHash" firestore:"destinationHash"`
	DirectActivityModeHash    int                       `json:"directActivityModeHash" firestore:"directActivityModeHash"`
	DirectActivityModeType    int                       `json:"directActivityModeType" firestore:"directActivityModeType"`
	DisplayProperties         ActivityDisplayProperties `json:"displayProperties" firestore:"displayProperties"`
	Hash                      int                       `json:"hash" firestore:"hash"`
	Index                     int                       `json:"index" firestore:"index"`
	InheritFromFreeRoam       bool                      `json:"inheritFromFreeRoam" firestore:"inheritFromFreeRoam"`
	InsertionPoints           []any                     `json:"insertionPoints" firestore:"insertionPoints"`
	IsPlaylist                bool                      `json:"isPlaylist" firestore:"isPlaylist"`
	IsPvP                     bool                      `json:"isPvP" firestore:"isPvP"`
	Matchmaking               ActivityMatchmaking       `json:"matchmaking" firestore:"matchmaking"`
	Modifiers                 []any                     `json:"modifiers" firestore:"modifiers"`
	OptionalUnlockStrings     []any                     `json:"optionalUnlockStrings" firestore:"optionalUnlockStrings"`
	OriginalDisplayProperties ActivityDisplayProperties `json:"originalDisplayProperties" firestore:"originalDisplayProperties"`
	PgcrImage                 string                    `json:"pgcrImage" firestore:"pgcrImage"`
	PlaceHash                 int                       `json:"placeHash" firestore:"placeHash"`
	PlaylistItems             []any                     `json:"playlistItems" firestore:"playlistItems"`
	Redacted                  bool                      `json:"redacted" firestore:"redacted"`
	ReleaseIcon               string                    `json:"releaseIcon" firestore:"releaseIcon"`
	ReleaseTime               int                       `json:"releaseTime" firestore:"releaseTime"`
	Rewards                   []any                     `json:"rewards" firestore:"rewards"`
	SuppressOtherRewards      bool                      `json:"suppressOtherRewards" firestore:"suppressOtherRewards"`
	Tier                      int                       `json:"tier" firestore:"tier"`
}

type ActivityDisplayProperties struct {
	Description string `json:"description" firestore:"description"`
	HasIcon     bool   `json:"hasIcon" firestore:"hasIcon"`
	Icon        string `json:"icon" firestore:"icon"`
	Name        string `json:"name" firestore:"name"`
}

type ActivityMatchmaking struct {
	IsMatchmade          bool `json:"isMatchmade" firestore:"isMatchmade"`
	MaxParty             int  `json:"maxParty" firestore:"maxParty"`
	MaxPlayers           int  `json:"maxPlayers" firestore:"maxPlayers"`
	MinParty             int  `json:"minParty" firestore:"minParty"`
	RequiresGuardianOath bool `json:"requiresGuardianOath" firestore:"requiresGuardianOath"`
}

type PerkDefinition struct {
	Hash              int64                       `json:"hash" firestore:"hash"`
	Index             int                         `json:"index" firestore:"index"`
	DisplayProperties DamageTypeDisplayProperties `json:"displayProperties" firestore:"displayProperties"`
	IsDisplayable     bool                        `json:"isDisplayable" firestore:"isDisplayable"`
	DamageType        int                         `json:"damageType" firestore:"damageType"`
	DamageTypeHash    int64                       `json:"damageTypeHash" firestore:"damageTypeHash"`
	Redacted          bool                        `json:"redacted" firestore:"redacted"`
	Blacklisted       bool                        `json:"blacklisted" firestore:"blacklisted"`
}

type DamageTypeDisplayProperties struct {
	Name          string         `json:"name" firestore:"name"`
	Description   string         `json:"description" firestore:"description"`
	Icon          string         `json:"icon" firestore:"icon"`
	IconSequences []IconSequence `json:"iconSequences" firestore:"iconSequences"`
	HasIcon       bool           `json:"hasIcon" firestore:"hasIcon"`
}

type StatDefinition struct {
	Hash              int64                 `json:"hash" firestore:"hash"`
	Index             int                   `json:"index" firestore:"index"`
	DisplayProperties StatDisplayProperties `json:"displayProperties" firestore:"displayProperties"`
	AggregationType   int                   `json:"aggregationType" firestore:"aggregationType"`
	HasComputedBlock  bool                  `json:"hasComputedBlock" firestore:"hasComputedBlock"`
	StatCategory      int                   `json:"statCategory" firestore:"statCategory"`
	Interpolate       bool                  `json:"interpolate" firestore:"interpolate"`
	Redacted          bool                  `json:"redacted" firestore:"redacted"`
	Blacklisted       bool                  `json:"blacklisted" firestore:"blacklisted"`
}

type StatDisplayProperties struct {
	Name          string         `json:"name" firestore:"name"`
	Description   string         `json:"description" firestore:"description"`
	Icon          string         `json:"icon" firestore:"icon"`
	IconSequences []IconSequence `json:"iconSequences" firestore:"iconSequences"`
	HasIcon       bool           `json:"hasIcon" firestore:"hasIcon"`
}

type IconSequence struct {
	Frames []string `json:"frames" firestore:"frames"`
}

type DamageType struct {
	DisplayProperties   DamageDisplayProperties `json:"displayProperties" firestore:"displayProperties"`
	TransparentIconPath string                  `json:"transparentIconPath" firestore:"transparentIconPath"`
	ShowIcon            bool                    `json:"showIcon" firestore:"showIcon"`
	EnumValue           int                     `json:"enumValue" firestore:"enumValue"`
	Color               DamageColor             `json:"color" firestore:"color"`
	Hash                int64                   `json:"hash" firestore:"hash"`
	Index               int                     `json:"index" firestore:"index"`
	Redacted            bool                    `json:"redacted" firestore:"redacted"`
	Blacklisted         bool                    `json:"blacklisted" firestore:"blacklisted"`
}
type DamageDisplayProperties struct {
	Description string `json:"description" firestore:"description"`
	Name        string `json:"name" firestore:"name"`
	Icon        string `json:"icon" firestore:"icon"`
	HasIcon     bool   `json:"hasIcon" firestore:"hasIcon"`
}

type DamageColor struct {
	Red   int `json:"red" firestore:"red"`
	Green int `json:"green" firestore:"green"`
	Blue  int `json:"blue" firestore:"blue"`
	Alpha int `json:"alpha" firestore:"alpha"`
}

type AuthResponse struct {
	AccessToken      string `json:"access_token" firestore:"accessToken"`
	TokenType        string `json:"token_type" firestore:"tokenType"`
	ExpiresIn        int    `json:"expires_in" firestore:"expiresIn"`
	RefreshToken     string `json:"refresh_token" firestore:"refreshToken"`
	RefreshExpiresIn int    `json:"refresh_expires_in" firestore:"refreshExpiresIn"`
	MembershipID     string `json:"membership_id" firestore:"membershipId"`
}

type RaceDisplayProperties struct {
	Description string `json:"description" firestore:"description"`
	HasIcon     bool   `json:"hasIcon" firestore:"hasIcon"`
	Name        string `json:"name" firestore:"name"`
}

type GenderedRaceNames struct {
	Female string `json:"female" firestore:"female"`
	Male   string `json:"male" firestore:"male"`
}

type RaceDefinition struct {
	Blacklisted                   bool                  `json:"blacklisted" firestore:"blacklisted"`
	DisplayProperties             RaceDisplayProperties `json:"displayProperties" firestore:"displayProperties"`
	GenderedRaceNames             GenderedRaceNames     `json:"genderedRaceNames" firestore:"genderedRaceNames"`
	GenderedRaceNamesByGenderHash GenderedRaceNames     `json:"genderedRaceNamesByGenderHash" firestore:"genderedRaceNamesByGenderHash"`
	Hash                          float64               `json:"hash" firestore:"hash"`
	Index                         int                   `json:"index" firestore:"index"`
	RaceType                      int                   `json:"raceType" firestore:"raceType"`
	Redacted                      bool                  `json:"redacted" firestore:"redacted"`
}

type RecordDefinition struct {
	DisplayProperties struct {
		Description   string `json:"description" firestore:"description"`
		Name          string `json:"name" firestore:"name"`
		Icon          string `json:"icon" firestore:"icon"`
		IconSequences []struct {
			Frames []string `json:"frames" firestore:"frames"`
		} `json:"iconSequences" firestore:"iconSequences"`
		HasIcon bool `json:"hasIcon" firestore:"hasIcon"`
	} `json:"displayProperties" firestore:"displayProperties"`
	Scope                int   `json:"scope" firestore:"scope"`
	ObjectiveHashes      []int `json:"objectiveHashes" firestore:"objectiveHashes"`
	RecordValueStyle     int   `json:"recordValueStyle" firestore:"recordValueStyle"`
	ForTitleGilding      bool  `json:"forTitleGilding" firestore:"forTitleGilding"`
	ShouldShowLargeIcons bool  `json:"shouldShowLargeIcons" firestore:"shouldShowLargeIcons"`
	TitleInfo            struct {
		HasTitle       bool `json:"hasTitle" firestore:"hasTitle"`
		TitlesByGender struct {
			Male   string `json:"Male" firestore:"male"`
			Female string `json:"Female" firestore:"female"`
		} `json:"titlesByGender" firestore:"titlesByGender"`
		TitlesByGenderHash struct {
			Num2204441813 string `json:"2204441813" firestore:"num2204441813"`
			Num3111576190 string `json:"3111576190" firestore:"num3111576190"`
		} `json:"titlesByGenderHash" firestore:"titlesByGenderHash"`
		GildingTrackingRecordHash int64 `json:"gildingTrackingRecordHash" firestore:"gildingTrackingRecordHash"`
	} `json:"titleInfo" firestore:"titleInfo"`
	CompletionInfo struct {
		PartialCompletionObjectiveCountThreshold int  `json:"partialCompletionObjectiveCountThreshold" firestore:"partialCompletionObjectiveCountThreshold"`
		ScoreValue                               int  `json:"ScoreValue" firestore:"scoreValue"`
		ShouldFireToast                          bool `json:"shouldFireToast" firestore:"shouldFireToast"`
		ToastStyle                               int  `json:"toastStyle" firestore:"toastStyle"`
	} `json:"completionInfo" firestore:"completionInfo"`
	StateInfo struct {
		FeaturedPriority                int64  `json:"featuredPriority" firestore:"featuredPriority"`
		ObscuredName                    string `json:"obscuredName" firestore:"obscuredName"`
		ObscuredDescription             string `json:"obscuredDescription" firestore:"obscuredDescription"`
		CompleteUnlockHash              int    `json:"completeUnlockHash" firestore:"completeUnlockHash"`
		ClaimedUnlockHash               int    `json:"claimedUnlockHash" firestore:"claimedUnlockHash"`
		CompletedCounterUnlockValueHash int    `json:"completedCounterUnlockValueHash" firestore:"completedCounterUnlockValueHash"`
	} `json:"stateInfo" firestore:"stateInfo"`
	Requirements struct {
		EntitlementUnavailableMessage string `json:"entitlementUnavailableMessage" firestore:"entitlementUnavailableMessage"`
	} `json:"requirements" firestore:"requirements"`
	ExpirationInfo struct {
		HasExpiration bool   `json:"hasExpiration" firestore:"hasExpiration"`
		Description   string `json:"description" firestore:"description"`
	} `json:"expirationInfo" firestore:"expirationInfo"`
	IntervalInfo struct {
		IntervalObjectives                   []interface{} `json:"intervalObjectives" firestore:"intervalObjectives"`
		IntervalRewards                      []interface{} `json:"intervalRewards" firestore:"intervalRewards"`
		OriginalObjectiveArrayInsertionIndex int           `json:"originalObjectiveArrayInsertionIndex" firestore:"originalObjectiveArrayInsertionIndex"`
		IsIntervalVersionedFromNormalRecord  bool          `json:"isIntervalVersionedFromNormalRecord" firestore:"isIntervalVersionedFromNormalRecord"`
	} `json:"intervalInfo" firestore:"intervalInfo"`
	RewardItems                       []interface{} `json:"rewardItems" firestore:"rewardItems"`
	AnyRewardHasConditionalVisibility bool          `json:"anyRewardHasConditionalVisibility" firestore:"anyRewardHasConditionalVisibility"`
	RecordTypeName                    string        `json:"recordTypeName" firestore:"recordTypeName"`
	PresentationNodeType              int           `json:"presentationNodeType" firestore:"presentationNodeType"`
	TraitIds                          []interface{} `json:"traitIds" firestore:"traitIds"`
	TraitHashes                       []interface{} `json:"traitHashes" firestore:"traitHashes"`
	ParentNodeHashes                  []interface{} `json:"parentNodeHashes" firestore:"parentNodeHashes"`
	Hash                              int           `json:"hash" firestore:"hash"`
	Index                             int           `json:"index" firestore:"index"`
	Redacted                          bool          `json:"redacted" firestore:"redacted"`
	Blacklisted                       bool          `json:"blacklisted" firestore:"blacklisted"`
}

type ModeDisplayProperties struct {
	Description string `json:"description" firestore:"description"`
	Name        string `json:"name" firestore:"name"`
	Icon        string `json:"icon" firestore:"icon"`
	HasIcon     bool   `json:"hasIcon" firestore:"hasIcon"`
}

type ActivityModeDefinition struct {
	DisplayProperties     ModeDisplayProperties `json:"displayProperties" firestore:"displayProperties"`
	PgcrImage             string                `json:"pgcrImage" firestore:"pgcrImage"`
	ModeType              int                   `json:"modeType" firestore:"modeType"`
	ActivityModeCategory  int                   `json:"activityModeCategory" firestore:"activityModeCategory"`
	IsTeamBased           bool                  `json:"isTeamBased" firestore:"isTeamBased"`
	Tier                  int                   `json:"tier" firestore:"tier"`
	IsAggregateMode       bool                  `json:"isAggregateMode" firestore:"isAggregateMode"`
	ParentHashes          []int64               `json:"parentHashes" firestore:"parentHashes"`
	FriendlyName          string                `json:"friendlyName" firestore:"friendlyName"`
	SupportsFeedFiltering bool                  `json:"supportsFeedFiltering" firestore:"supportsFeedFiltering"`
	Display               bool                  `json:"display" firestore:"display"`
	Order                 int                   `json:"order" firestore:"order"`
	Hash                  int64                 `json:"hash" firestore:"hash"`
	Index                 int                   `json:"index" firestore:"index"`
	Redacted              bool                  `json:"redacted" firestore:"redacted"`
	Blacklisted           bool                  `json:"blacklisted" firestore:"blacklisted"`
}

type ManifestResponse struct {
	Response struct {
		Version                  string `json:"version"`
		MobileAssetContentPath   string `json:"mobileAssetContentPath"`
		MobileGearAssetDataBases []struct {
			Version int    `json:"version"`
			Path    string `json:"path"`
		} `json:"mobileGearAssetDataBases"`
		MobileWorldContentPaths        ContentPaths                 `json:"mobileWorldContentPaths"`
		JsonWorldContentPaths          ContentPaths                 `json:"jsonWorldContentPaths"`
		JsonWorldComponentContentPaths map[string]map[string]string `json:"jsonWorldComponentContentPaths"`
	} `json:"Response"`
	ErrorCode       int               `json:"ErrorCode"`
	ThrottleSeconds int               `json:"ThrottleSeconds"`
	ErrorStatus     string            `json:"ErrorStatus"`
	Message         string            `json:"Message"`
	MessageData     map[string]string `json:"MessageData"`
}

type ManifestUpdate struct {
	Version      string
	ManifestURL  string
	ShouldUpdate bool
}
type ContentPaths struct {
	EN string `json:"en"`
	FR string `json:"fr"`
}

type Configuration struct {
	ManifestVersion string `json:"manifestVersion" firestore:"manifestVersion"`

	PlaceVersion            string `json:"placeVersion" firestore:"placeVersion"`
	ActivityVersion         string `json:"activityVersion" firestore:"activityVersion"`
	ClassVersion            string `json:"classVersion" firestore:"classVersion"`
	InventoryBucketVersion  string `json:"inventoryBucketVersion" firestore:"inventoryBucketVersion"`
	RaceVersion             string `json:"raceVersion" firestore:"raceVersion"`
	ItemCategoryVersion     string `json:"itemCategoryVersion" firestore:"itemCategoryVersion"`
	DamageVersion           string `json:"damageVersion" firestore:"damageVersion"`
	ActivityModeVersion     string `json:"activityModeVersion" firestore:"activityModeVersion"`
	StatDefinitionVersion   string `json:"statDefinitionVersion" firestore:"statDefinitionVersion"`
	ItemDefinitionVersion   string `json:"itemDefinitionVersion" firestore:"itemDefinitionVersion"`
	SandboxPerkVersion      string `json:"sandboxPerkVersion" firestore:"sandboxPerkVersion"`
	RecordDefinitionVersion string `json:"recordDefinitionVersion" firestore:"recordDefinitionVersion"`
	CrucibleMapVersion      string `json:"crucibleMapVersion" firestore:"crucibleMapVersion"`
}

type WeaponBucket = uint32

const (
	KineticBucket WeaponBucket = 1498876634
	EnergyBucket  WeaponBucket = 2465295065
	PowerBucket   WeaponBucket = 953998645
)

const (
	ConfigurationCollection = "configurations"
	DestinyDocument         = "destiny"
	ManifestObjectName      = "manifest.json"
	DestinyBucket           = "destiny"
	mntLocation             = "mnt/destiny/manifest.json"
	LocalManifestLocation   = "./manifest.json"
)

const Kinetic = 1498876634
const Energy = 2465295065
const Power = 953998645
const SubClass = 3284755031

type ArmorBucket = uint32

const (
	HelmetArmor    ArmorBucket = 3448274439
	GauntletsArmor ArmorBucket = 3551918588
	ChestArmor     ArmorBucket = 14239492
	LegArmor       ArmorBucket = 20886954
	ClassArmor     ArmorBucket = 1585787867
)

type RequestInfo = int32

const (
	CharactersCode      RequestInfo = 200
	CharactersEquipment RequestInfo = 205
	ItemInstanceCode    RequestInfo = 300
	ItemPerksCode       RequestInfo = 302
	ItemStatsCode       RequestInfo = 304
	ItemSocketsCode     RequestInfo = 305
	ItemCommonDataCode  RequestInfo = 307
	TransitoryCode      RequestInfo = 1000
)

type ManifestCollection string

const (
	PlaceCollection            ManifestCollection = "d2Places"
	ActivityCollection         ManifestCollection = "d2Activities"
	ClassCollection            ManifestCollection = "d2Classes"
	InventoryBucketCollection  ManifestCollection = "d2InventoryBuckets"
	RaceCollection             ManifestCollection = "d2Races"
	ItemCategoryCollection     ManifestCollection = "d2ItemCategories"
	DamageCollection           ManifestCollection = "d2DamageTypes"
	ActivityModeCollection     ManifestCollection = "d2ActivityModes"
	StatDefinitionCollection   ManifestCollection = "d2StatDefinitions"
	ItemDefinitionCollection   ManifestCollection = "d2ItemDefinitions"
	SandboxPerkCollection      ManifestCollection = "d2SandboxPerks"
	RecordDefinitionCollection ManifestCollection = "d2RecordDefinitions"
	CrucibleMapCollection      ManifestCollection = "crucibleMaps"
)
