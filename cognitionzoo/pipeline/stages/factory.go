package stages

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/pipeline"
	execution "codeburg.org/lexbit/relurpify/execution"
)

// CodingStageFactory builds the first concrete coding pipeline for a task.
type CodingStageFactory struct{}

func (f CodingStageFactory) StagesForTask(task *execution.Task) ([]pipeline.Stage, error) {
	if task != nil && execution.TaskType(task.Type) == execution.TaskTypeExplain {
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
