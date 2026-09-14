package app

// Where WhatsApp keeps its messages inside an iPhone backup.
//
// The domain is Apple's name for the group container the app shares with its
// extensions, and the file is the Core Data store inside it. Both are WhatsApp's
// choices rather than ours, and both are what every part of this program looks for.
const (
	WhatsAppDomain = "AppDomainGroup-group.net.whatsapp.WhatsApp.shared"
	ChatStorage    = "ChatStorage.sqlite"
)
