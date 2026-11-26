package desktop

import "context"

// ModeDTO is the IPC projection of a built-in Mode. A model is not part of a
// Mode: it is selected per run through StartRunRequestDTO.ModelRef.
type ModeDTO struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Instructions      string   `json:"instructions"`
	ToolsetIDs        []string `json:"toolsetIds"`
	SkillIDs          []string `json:"skillIds"`
	Workspace         string   `json:"workspace"`
	PermissionProfile string   `json:"permissionProfile"`
	ContextPolicy     string   `json:"contextPolicy"`
	Available         bool     `json:"available"`
	Phase             int      `json:"phase"`
}

type SwitchEstimateRequestDTO struct {
	ConversationID string `json:"conversationId"`
	ModelRef       string `json:"modelRef"`
	ModeID         string `json:"modeId"`
}

type SwitchEstimateDTO struct {
	EstimatedTokens int64 `json:"estimatedTokens"`
	Warn            bool  `json:"warn"`
	Estimated       bool  `json:"estimated"`
}

func (s *Service) ListModes(ctx context.Context) []ModeDTO {
	modes := s.svc.ListModes(ctx)
	out := make([]ModeDTO, 0, len(modes))
	for _, mode := range modes {
		out = append(out, ModeDTO{
			ID: mode.ID, Name: mode.Name, Instructions: mode.Instructions,
			ToolsetIDs: append([]string(nil), mode.ToolsetIDs...),
			SkillIDs:   append([]string(nil), mode.SkillIDs...),
			Workspace:  mode.Workspace, PermissionProfile: mode.PermissionProfile,
			ContextPolicy: mode.ContextPolicy, Available: mode.Available, Phase: mode.Phase,
		})
	}
	return out
}

func (s *Service) EstimateSwitch(ctx context.Context, req SwitchEstimateRequestDTO) (SwitchEstimateDTO, error) {
	estimate, err := s.svc.EstimateSwitch(ctx, req.ConversationID, req.ModelRef, req.ModeID)
	if err != nil {
		return SwitchEstimateDTO{}, err
	}
	return SwitchEstimateDTO{
		EstimatedTokens: estimate.EstimatedTokens,
		Warn:            estimate.Warn,
		Estimated:       estimate.Estimated,
	}, nil
}
