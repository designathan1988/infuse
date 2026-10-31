package main

// Temporary aliases for the extraction of internal packages. They keep the root package compiling
// while symbols move out, and are removed once the last domain leaves (stage 9).

import "infuse/internal/core"

type Block = core.Block
type Message = core.Message
type ToolSpec = core.ToolSpec
type Usage = core.Usage
type InlineImage = core.InlineImage
type TaskItem = core.TaskItem
type CheckpointFile = core.CheckpointFile
type Event = core.Event
type Attachment = core.Attachment
type Msg = core.Msg

func M(id string, kv ...any) Msg { return core.M(id, kv...) }

func registerMessages(m map[string]map[string]string) { core.RegisterMessages(m) }

func displayFor(lang, content string, msgs []Msg) string {
	return core.DisplayFor(lang, content, msgs)
}
