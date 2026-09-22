package dto

import "time"

// ChainLink describes one release-chain relation of an entity, e.g. the batch
// a sample was received under or the method version a review depends on.
type ChainLink struct {
	Kind   string `json:"kind"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Exists bool   `json:"exists"`
}

// ChainCheck is a live evaluation of one release constraint so the workbench
// can show why an action would currently be allowed or blocked.
type ChainCheck struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason"`
}

// ChainBlock is a persisted record of a rejected release attempt. Entries come
// from the append-only audit log so they survive page refreshes.
type ChainBlock struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Reason string    `json:"reason"`
}

// DisposalSnapshot preserves the method, batch and reason recorded when a
// sample was disposed.
type DisposalSnapshot struct {
	Reason     string     `json:"reason"`
	BatchCode  string     `json:"batchCode"`
	MethodCode string     `json:"methodCode"`
	DisposedAt *time.Time `json:"disposedAt"`
}

// ChainView aggregates links, live checks and persisted blocks for one entity.
type ChainView struct {
	EntityType string            `json:"entityType"`
	EntityID   uint              `json:"entityId"`
	Code       string            `json:"code"`
	Status     string            `json:"status"`
	Links      []ChainLink       `json:"links"`
	Checks     []ChainCheck      `json:"checks"`
	Blocks     []ChainBlock      `json:"blocks"`
	Disposal   *DisposalSnapshot `json:"disposal,omitempty"`
}
