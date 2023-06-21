package main

type CleanMissionStatus struct {
	Phase string `json:"phase"`
}

type RoombaRegionParams struct {
	NoAutoPasses bool `json:"noAutoPasses"`
	TwoPass      bool `json:"twoPass"`
}

type RoombaRegion struct {
	RegionId string             `json:"region_id"`
	Type     string             `json:"type"`
	Params   RoombaRegionParams `json:"params"`
}

type Command struct {
	Command   string         `json:"command,omitempty"`
	Initiator string         `json:"initiator,omitempty"`
	Time      int            `json:"time,omitempty"`
	Regions   []RoombaRegion `json:"regions,omitempty"`
	PmapId    string         `json:"pmap_id,omitempty"`
}

type Bin struct {
	Present bool `json:"present"`
	Full    bool `json:"full"`
}

type MapMap map[string]string

type Reported struct {
	Name               *string             `json:"name"`
	BatteryPercent     *int                `json:"batPct"`
	SKU                *string             `json:"sku"`
	SoftwareVer        *string             `json:"softwareVer"`
	LidOpen            *bool               `json:"lidOpen,omitempty"`
	TankPresent        *bool               `json:"tankPresent,omitempty"`
	TankLvl            *int                `json:"tankLvl,omitempty"`
	DetectedPad        *string             `json:"detectedPad,omitempty"`
	Bin                *Bin                `json:"bin,omitempty"`
	CleanMissionStatus *CleanMissionStatus `json:"cleanMissionStatus,omitempty"`
	LastCommand        *Command            `json:"lastCommand,omitempty"`
	Maps               *[]MapMap           `json:"pmaps,omitempty"`
}

type State struct {
	Reported Reported `json:"reported"`
}

type RoombaMessage struct {
	State State `json:"state"`
}
