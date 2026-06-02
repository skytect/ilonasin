package server

import (
	"sort"

	"ilonasin/internal/provider"
)

type modelsResponse struct {
	Object string                    `json:"object"`
	Data   []modelListItem           `json:"data"`
	Models []provider.CodexModelInfo `json:"models"`
}

type modelListItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

func modelsResponseFromMetadata(rows []provider.ModelMetadata) modelsResponse {
	rows = sortedModelMetadata(rows)
	data := make([]modelListItem, 0, len(rows))
	codexModels := make([]provider.CodexModelInfo, 0, len(rows))
	for _, row := range rows {
		id := row.ProviderInstanceID + "/" + row.ModelID
		data = append(data, modelListItem{
			ID:      id,
			Object:  "model",
			OwnedBy: row.ProviderInstanceID,
		})
		if codex, ok := provider.CodexModelInfoFromMetadata(row, id); ok {
			codexModels = append(codexModels, codex)
		}
	}
	return modelsResponse{Object: "list", Data: data, Models: codexModels}
}

func sortedModelMetadata(rows []provider.ModelMetadata) []provider.ModelMetadata {
	out := append([]provider.ModelMetadata(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ProviderInstanceID != out[j].ProviderInstanceID {
			return out[i].ProviderInstanceID < out[j].ProviderInstanceID
		}
		return out[i].ModelID < out[j].ModelID
	})
	return out
}
