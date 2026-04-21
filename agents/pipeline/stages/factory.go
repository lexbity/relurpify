package stages

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/pipeline"
)

// CodingStageFactory builds the first concrete coding pipeline for a task.
type CodingStageFactory struct{}

func (f CodingStageFactory) StagesForTask(task *core.Task) ([]pipeline.Stage, error) {
	if task != nil && task.Type == core.TaskTypeAnalysis {
		return []pipeline.Stage{
			&ExploreStage{Task: task},
			&VerifyStage{Task: task},
		}, nil
	}
	return []pipeline.Stage{
		&ExploreStage{Task: task},
		&AnalyzeStage{Task: task},
		&PlanStage{Task: task},
		&CodeStage{Task: task},
		&VerifyStage{Task: task},
	}, nil
}
