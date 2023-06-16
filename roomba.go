package main

type CleanMissionStatus struct {
	Phase string `json:"phase"`
}

type LastCommand struct {
	Command   string `json:"command"`
	Initiator string `json:"initiator"`
	Time      int    `json:"time"`
}

type Bin struct {
	Present bool `json:"present"`
	Full    bool `json:"full"`
}

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
	LastCommand        *LastCommand        `json:"lastCommand,omitempty"`
}

type State struct {
	Reported Reported `json:"reported"`
}

type RoombaMessage struct {
	State State `json:"state"`
}
