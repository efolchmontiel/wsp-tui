package jidutil

import "go.mau.fi/whatsmeow/types"

// IsGroup reports a real WhatsApp group chat (g.us).
func IsGroup(jid types.JID) bool {
	return jid.Server == types.GroupServer
}

// IsStatusBroadcast is the status/stories thread (status@broadcast).
func IsStatusBroadcast(jid types.JID) bool {
	return jid.Server == "broadcast" && (jid.User == "status" || jid.String() == "status@broadcast")
}

// IsBroadcastList is any broadcast JID (including status).
func IsBroadcastList(jid types.JID) bool {
	return jid.Server == "broadcast"
}
