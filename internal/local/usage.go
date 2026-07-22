package local

import (
	"encoding/json"
	"fmt"
)

// usage.go — the D7 usage-block marker for local calls (brief R18). Every
// local call writes ONE checkpoint-row usage block on the consuming run,
// carrying token counts (so the metering read model tier-1 measures the row —
// the field names match the S02.4 usage schema, metering.EngineUsage) PLUS a
// `local` marker the metering fold HONORS: it overrides the row's lane to
// "local" (lane is otherwise joined from runs — S02.2; §8 reading 4) and
// carries {duty alias, resolved model, model file sha256, engine build} for
// the zero-allowance itemization and the S12.9/S12.10 (duty, model hash,
// engine build) keying (B4-7).
//
// This is a JSON WIRE CONTRACT read by internal/metering (fold override + $0
// pricing); neither package imports the other (CONVENTIONS §26 import wall).
// The contract: {"input_tokens":N,"output_tokens":M,"local":{"lane":"local",
// "duty":…,"model":…,"model_sha256":…,"engine_build":…}}.

// LocalMarker is the `local` sub-object of a local call's D7 usage block.
type LocalMarker struct {
	Lane        string `json:"lane"`         // always LaneLocal — the fold's lane override
	Duty        string `json:"duty"`         // the S12.4 duty alias
	Model       string `json:"model"`        // the resolved seat model
	ModelSHA256 string `json:"model_sha256"` // the manifest model hash (B4-7 keys on it)
	EngineBuild string `json:"engine_build"` // the llama.cpp b-tag (LlamaCppPin)
}

// usageBlock is a local call's full D7 usage block: tokens + the marker.
type usageBlock struct {
	InputTokens  int64        `json:"input_tokens"`
	OutputTokens int64        `json:"output_tokens"`
	Local        *LocalMarker `json:"local"`
}

// UsageJSON builds the D7 usage block for a local call (R18). engineBuild is
// LlamaCppPin; modelSHA256 is the manifest model hash (empty until the
// bring-up pull, honest either way).
func UsageJSON(duty, model, modelSHA256, engineBuild string, inTok, outTok int64) (json.RawMessage, error) {
	b, err := json.Marshal(usageBlock{
		InputTokens:  inTok,
		OutputTokens: outTok,
		Local:        &LocalMarker{Lane: LaneLocal, Duty: duty, Model: model, ModelSHA256: modelSHA256, EngineBuild: engineBuild},
	})
	if err != nil {
		return nil, fmt.Errorf("local: marshal usage block: %w", err)
	}
	return b, nil
}
