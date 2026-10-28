package sync

import (
	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/pihole"
)

const (
	ModeFull      = "full"
	ModeSelective = "selective"
	ModeMixed     = "mixed"
)

type Settings struct {
	Profile    string
	Mode       string
	RunGravity bool
	Gravity    *config.GravitySettings
	Config     *config.ConfigSettings
}

type ReplicaPlan struct {
	Client   pihole.Client
	Settings *Settings
}

type Plan struct {
	Replicas []ReplicaPlan
}

func NewSettings(profile *config.Profile) *Settings {
	if profile == nil {
		profile = &config.Profile{}
	}

	settings := Settings{
		Profile:    profile.Name,
		Mode:       ModeSelective,
		RunGravity: profile.RunGravity,
		Gravity:    profile.GravitySettings,
		Config:     profile.ConfigSettings,
	}

	if profile.FullSync {
		settings.Mode = ModeFull
		settings.Gravity = newFullSyncGravitySettings()
		settings.Config = newFullSyncConfigSettings()
	}

	if settings.Config == nil {
		settings.Config = newDisabledConfigSettings()
	}

	return &settings
}

func (p Plan) mode() string {
	if len(p.Replicas) == 0 {
		return ModeSelective
	}

	mode := p.Replicas[0].Settings.Mode
	for _, replica := range p.Replicas[1:] {
		if replica.Settings.Mode != mode {
			return ModeMixed
		}
	}

	return mode
}

func newFullSyncConfigSettings() *config.ConfigSettings {
	return &config.ConfigSettings{
		DNS:       config.NewConfigSetting(true, nil, nil),
		DHCP:      config.NewConfigSetting(true, nil, nil),
		NTP:       config.NewConfigSetting(true, nil, nil),
		Resolver:  config.NewConfigSetting(true, nil, nil),
		Database:  config.NewConfigSetting(true, nil, nil),
		Webserver: config.NewConfigSetting(false, nil, nil),
		Files:     config.NewConfigSetting(false, nil, nil),
		Misc:      config.NewConfigSetting(true, nil, nil),
		Debug:     config.NewConfigSetting(true, nil, nil),
	}
}

func newFullSyncGravitySettings() *config.GravitySettings {
	return &config.GravitySettings{
		DHCPLeases:        true,
		Group:             true,
		Adlist:            true,
		AdlistByGroup:     true,
		Domainlist:        true,
		DomainlistByGroup: true,
		Client:            true,
		ClientByGroup:     true,
	}
}

func newDisabledConfigSettings() *config.ConfigSettings {
	return &config.ConfigSettings{
		DNS:       config.NewConfigSetting(false, nil, nil),
		DHCP:      config.NewConfigSetting(false, nil, nil),
		NTP:       config.NewConfigSetting(false, nil, nil),
		Resolver:  config.NewConfigSetting(false, nil, nil),
		Database:  config.NewConfigSetting(false, nil, nil),
		Webserver: config.NewConfigSetting(false, nil, nil),
		Files:     config.NewConfigSetting(false, nil, nil),
		Misc:      config.NewConfigSetting(false, nil, nil),
		Debug:     config.NewConfigSetting(false, nil, nil),
	}
}
