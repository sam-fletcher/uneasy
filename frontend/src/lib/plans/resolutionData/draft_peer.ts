// draft_peer.ts — DraftPeer, the shared shape for a peer who exists in the
// fiction but owns no asset row yet. Mirrors game/draft_peer.go.
//
// Two plans hold drafts: Make Introductions names peers before the dice are
// cast, and Host Festivity puts them in the middle of the party for any guest
// to claim. Nothing is in a retinue until the draft materializes — arrival is
// creation (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md, D4). The two share this
// shape, not their lifecycles (D8).

export interface DraftPeer {
	id: string;
	name: string;
	marginalia?: string;
	creator_id: number;
}
