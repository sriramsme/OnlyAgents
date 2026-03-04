package core

// Capability represents a connector capability type
type Capability string

const (
	// Email capabilities
	CapabilityEmail Capability = "email"

	// Calendar capabilities
	CapabilityCalendar Capability = "calendar"

	// Web capabilities
	CapabilityWebSearch Capability = "web_search"
	CapabilityWebFetch  Capability = "web_fetch"

	// Task management
	CapabilityTasks Capability = "tasks"

	// Storage capabilities
	// CapabilityStorage Capability = "storage"

	// Notes capabilities
	CapabilityNotes Capability = "notes"

	CapabilityReminders Capability = "reminders"

	// Communication
	// CapabilitySMS Capability = "sms"
)

// String returns the string representation
func (c Capability) String() string {
	return string(c)
}

// AllCapabilities returns all defined capabilities
func AllCapabilities() []Capability {
	return []Capability{
		CapabilityEmail,
		CapabilityCalendar,
		CapabilityWebSearch,
		CapabilityWebFetch,
		CapabilityTasks,
		CapabilityNotes,
	}
}

func AllCapabilityStrings() []string {
	var caps []string
	for _, cap := range AllCapabilities() {
		caps = append(caps, cap.String())
	}
	return caps
}
