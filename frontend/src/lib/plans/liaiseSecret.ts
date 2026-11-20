// liaiseSecret.ts
// The header the server puts in front of a Clandestinely Liaise secret.
//
// In Secrets We Keep each player writes the meeting on the underside of one of their
// assets. The server stores it behind a line saying who the holder met, "Met bob's Ser
// Ott", and the panel shows that line in the box's hint so the player doesn't type the
// name twice. A secret outlives the meeting: whoever later looks under, takes or breaks
// the asset reads it, and without the name the words can mean nothing. Who met whom is
// public from the preparation, so the header reveals nothing new.
//
// This mirrors clSecretHeader in handler/plan_clandestinely_liaise_phases.go. The server
// is authoritative; keep the two in step. The peer is named as it is now, and a peer
// that can't be found leaves the player's name alone.

/** The header for a secret whose holder met `playerName`, who brought `peerName`. */
export function liaiseSecretHeader(playerName: string, peerName?: string | null): string {
	return peerName ? `Met ${playerName}'s ${peerName}` : `Met ${playerName}`;
}
