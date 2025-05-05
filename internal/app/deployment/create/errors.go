package create

import "fmt"

type DeploymentFailed struct {
	Err error
}

func NewDeploymentFailed(err error) error {
	return DeploymentFailed{Err: err}
}

func (e DeploymentFailed) Error() string {
	return fmt.Sprintf("deployment failed: %v", e.Err)
}

func (e DeploymentFailed) Unwrap() error {
	return e.Err
}
