package config

import "fmt"

const DefaultProfileName = "default"

type Profile struct {
	Name            string
	FullSync        bool
	RunGravity      bool
	GravitySettings *GravitySettings
	ConfigSettings  *ConfigSettings
}

func (s *Sync) Profile() *Profile {
	return &Profile{
		Name:            DefaultProfileName,
		FullSync:        s.FullSync,
		RunGravity:      s.RunGravity,
		GravitySettings: s.GravitySettings,
		ConfigSettings:  s.ConfigSettings,
	}
}

func (p *Profile) String() string {
	return fmt.Sprintf(
		"{Name:%s FullSync:%t RunGravity:%t GravitySettings:%v ConfigSettings:%v}",
		p.Name,
		p.FullSync,
		p.RunGravity,
		p.GravitySettings,
		p.ConfigSettings,
	)
}
