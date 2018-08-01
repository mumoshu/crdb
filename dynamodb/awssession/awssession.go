package awssession

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
)

func New(debug bool) (*session.Session, error) {
	awsConfig := aws.NewConfig().
		WithCredentialsChainVerboseErrors(true)

	if debug {
		awsConfig = awsConfig.WithLogLevel(aws.LogDebug)
	}

	session, err := newAwsSessionFromConfig(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to establish aws session: %v", err)
	}
	return session, nil
}

func newAwsSessionFromConfig(config *aws.Config) (*session.Session, error) {
	return session.NewSessionWithOptions(session.Options{
		Config: *config,
		// This seems to be required for AWS_SDK_LOAD_CONFIG
		SharedConfigState: session.SharedConfigEnable,
		// This seems to be required by MFA
		AssumeRoleTokenProvider: stscreds.StdinTokenProvider,
	})
}
