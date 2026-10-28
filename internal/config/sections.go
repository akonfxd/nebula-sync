package config

const (
	SectionDNS       = "dns"
	SectionDHCP      = "dhcp"
	SectionNTP       = "ntp"
	SectionResolver  = "resolver"
	SectionDatabase  = "database"
	SectionWebserver = "webserver"
	SectionFiles     = "files"
	SectionMisc      = "misc"
	SectionDebug     = "debug"
)

const (
	GravityDHCPLeases        = "dhcpLeases"
	GravityGroup             = "group"
	GravityAdlist            = "adlist"
	GravityAdlistByGroup     = "adlistByGroup"
	GravityDomainlist        = "domainlist"
	GravityDomainlistByGroup = "domainlistByGroup"
	GravityClient            = "client"
	GravityClientByGroup     = "clientByGroup"
)

const (
	webserverNotSyncable = "the webserver section carries the api password, syncing it would lock you out of every replica"
	filesNotSyncable     = "file paths describe the host, not the configuration"
)

// SectionInfo describes one Pi-hole configuration section as the api exposes it.
type SectionInfo struct {
	Name     string
	Syncable bool
	Reason   string
}

// Sections lists every section in the order the api and the database use.
func Sections() []SectionInfo {
	return []SectionInfo{
		{Name: SectionDNS, Syncable: true},
		{Name: SectionDHCP, Syncable: true},
		{Name: SectionNTP, Syncable: true},
		{Name: SectionResolver, Syncable: true},
		{Name: SectionDatabase, Syncable: true},
		{Name: SectionWebserver, Syncable: false, Reason: webserverNotSyncable},
		{Name: SectionFiles, Syncable: false, Reason: filesNotSyncable},
		{Name: SectionMisc, Syncable: true},
		{Name: SectionDebug, Syncable: true},
	}
}

// SectionSyncable reports whether a section may be enabled on a profile.
func SectionSyncable(name string) bool {
	for _, section := range Sections() {
		if section.Name == name {
			return section.Syncable
		}
	}

	return false
}

// SectionField pairs a section name with the field holding its setting.
type SectionField struct {
	Name  string
	Value **ConfigSetting
}

// Fields exposes every section as an addressable field so callers can read and write them by name.
func (cs *ConfigSettings) Fields() []SectionField {
	return []SectionField{
		{SectionDNS, &cs.DNS},
		{SectionDHCP, &cs.DHCP},
		{SectionNTP, &cs.NTP},
		{SectionResolver, &cs.Resolver},
		{SectionDatabase, &cs.Database},
		{SectionWebserver, &cs.Webserver},
		{SectionFiles, &cs.Files},
		{SectionMisc, &cs.Misc},
		{SectionDebug, &cs.Debug},
	}
}

// GravityTables lists the gravity tables in the order the api and the database use.
func GravityTables() []string {
	return []string{
		GravityDHCPLeases,
		GravityGroup,
		GravityAdlist,
		GravityAdlistByGroup,
		GravityDomainlist,
		GravityDomainlistByGroup,
		GravityClient,
		GravityClientByGroup,
	}
}

// Tables exposes every gravity flag as an addressable field so callers can read and write them by name.
func (gs *GravitySettings) Tables() map[string]*bool {
	return map[string]*bool{
		GravityDHCPLeases:        &gs.DHCPLeases,
		GravityGroup:             &gs.Group,
		GravityAdlist:            &gs.Adlist,
		GravityAdlistByGroup:     &gs.AdlistByGroup,
		GravityDomainlist:        &gs.Domainlist,
		GravityDomainlistByGroup: &gs.DomainlistByGroup,
		GravityClient:            &gs.Client,
		GravityClientByGroup:     &gs.ClientByGroup,
	}
}
