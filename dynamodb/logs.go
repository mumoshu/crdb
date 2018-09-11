// (c) 2018 Luca Grulla
// This file originates from https://github.com/lucagrulla/cw and I have made several tweaks it make it usable as a library

package dynamodb

import (
	"bytes"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/mumoshu/crdb/api"
	"github.com/mumoshu/crdb/dynamodb/awssession"
	"github.com/mumoshu/crdb/framework"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"time"
)

const SecondInMillis = 1000
const MinuteInMillis = 60 * SecondInMillis

type cwlogs struct {
	client    *cloudwatchlogs.CloudWatchLogs
	config    *api.Config
	namespace string
}

func NewLogs(configFile string, namespace string) (*cwlogs, error) {
	sess, err := awssession.New(os.Getenv("AWSDEBUG") != "")
	if err != nil {
		return nil, err
	}
	config, err := framework.LoadConfigFromYamlFile(configFile)
	if err != nil {
		return nil, err
	}
	return newLogs(config, namespace, sess)
}

func newLogs(config *api.Config, namespace string, sess *session.Session) (*cwlogs, error) {
	return &cwlogs{
		client:    cloudwatchlogs.New(sess),
		config:    config,
		namespace: namespace,
	}, nil
}

func createFilterLogEventsInput(logGroupName string, streamNames []*string, epochStartTime *int64) *cloudwatchlogs.FilterLogEventsInput {
	startTimeInt64 := epochStartTime
	params := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: &logGroupName,
		Interleaved:  aws.Bool(true),
	}
	if startTimeInt64 != nil {
		params.StartTime = startTimeInt64
	}

	if streamNames != nil {
		params.LogStreamNames = streamNames
	}

	return params
}

type eventCache struct {
	seen map[string]bool
	sync.RWMutex
}

func (c *eventCache) Has(eventID string) bool {
	c.RLock()
	defer c.RUnlock()
	return c.seen[eventID]
}

func (c *eventCache) Add(eventID string) {
	c.Lock()
	defer c.Unlock()
	c.seen[eventID] = true
}

func (c *eventCache) Size() int {
	c.RLock()
	defer c.RUnlock()
	return len(c.seen)
}

func (c *eventCache) Reset() {
	c.Lock()
	defer c.Unlock()
	c.seen = make(map[string]bool)
}

type logStreams struct {
	groupStreams []*string
	sync.RWMutex
}

func (s *logStreams) reset(groupStreams []*string) {
	s.Lock()
	defer s.Unlock()
	s.groupStreams = groupStreams
}

func (s *logStreams) get() []*string {
	s.Lock()
	defer s.Unlock()
	return s.groupStreams
}

type ErrLogsNotFound struct {
	msg string
}

func (e *ErrLogsNotFound) Error() string {
	return e.msg
}

func (c *cwlogs) Read(resource, name string, since time.Duration, follow bool) error {
	logsCh, errCh := c.read(resource, name, since, follow)
	interrupts := make(chan os.Signal, 1)
	defer close(interrupts)
	signal.Notify(interrupts, os.Interrupt)
	var err error
	for logsCh != nil || errCh != nil {
		select {
		case <-interrupts:
			fmt.Fprintln(os.Stderr, "interrupted")
			return nil
		case e := <-errCh:
			if e != nil {
				switch typed := e.(type) {
				case awserr.Error:
					if typed.Code() == cloudwatchlogs.ErrCodeResourceNotFoundException {
						err = &ErrLogsNotFound{fmt.Sprintf("log stream for resource=%s name=%s does not exist (yet)", resource, name)}
					} else {
						err = e
					}
				default:
					err = e
				}
			}
			errCh = nil
		case log, ok := <-logsCh:
			if !ok {
				logsCh = nil
			}
			if log != nil {
				fmt.Printf("%s", *log.Message)
			}
		default:
			time.Sleep(1000 * time.Millisecond)
		}
	}
	return err
}

func (c *cwlogs) read(resource, name string, since time.Duration, follow bool) (<-chan *cloudwatchlogs.FilteredLogEvent, <-chan error) {
	logGroup := fmt.Sprintf("%s%s-%s-%s", databasePrefix, c.config.Metadata.Name, c.namespace, resource)
	var startTime *time.Time
	if since.Nanoseconds() == 0 {
		startTime = nil
	} else {
		t := time.Now().Add(-since)
		startTime = &t
	}
	return c.readLogEvents(logGroup, name, follow, startTime)
}

func (c *cwlogs) Writer(resource, name string) (io.Writer, error) {
	r, w := io.Pipe()

	buf := new(bytes.Buffer)
	go func() {
		buf.ReadFrom(r)
		fmt.Fprint(w, "some text to be read\n")
		w.Close()
	}()

	return w, nil
}

func (c *cwlogs) WriteFile(resource, name string, file string) error {
	var rawInput []byte
	if file == "-" {
		var buf bytes.Buffer

		nr, err := io.Copy(&buf, os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %v", err)
		}
		rawInput = buf.Bytes()
		fmt.Fprintf(os.Stderr, "read %d byytes from stdin\n", nr)
	} else {
		raw, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}
		rawInput = raw
	}
	logStream := name
	logGroup := fmt.Sprintf("%s%s-%s-%s", databasePrefix, c.config.Metadata.Name, c.namespace, resource)
	out, err := c.client.DescribeLogGroups(&cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(logGroup),
	})
	if err != nil {
		return err
	}
	if len(out.LogGroups) == 0 {
		_, err := c.client.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroup),
		})
		if err != nil {
			return err
		}
	}
	describeStreamOut, err := c.client.DescribeLogStreams(&cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        aws.String(logGroup),
		LogStreamNamePrefix: aws.String(logStream),
	})
	outLogStreams := describeStreamOut.LogStreams
	if len(outLogStreams) == 0 {
		if _, err := c.client.CreateLogStream(&cloudwatchlogs.CreateLogStreamInput{
			LogGroupName:  aws.String(logGroup),
			LogStreamName: aws.String(logStream),
		}); err != nil {
			return err
		}

		describeStreamOut, err = c.client.DescribeLogStreams(&cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName:        aws.String(logGroup),
			LogStreamNamePrefix: aws.String(logStream),
		})
		outLogStreams = describeStreamOut.LogStreams
	}

	logEvents := []*cloudwatchlogs.InputLogEvent{
		{
			Message:   aws.String(string(rawInput)),
			Timestamp: aws.Int64(time.Now().Unix() * 1000),
		},
	}
	putInput := &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
		LogEvents:     logEvents,
	}
	firstLogStream := outLogStreams[0]
	seqToken := firstLogStream.UploadSequenceToken
	if seqToken != nil {
		putInput.SequenceToken = seqToken
	}
	_, putErr := c.client.PutLogEvents(putInput)
	if putErr != nil {
		return putErr
	}
	return nil
}

func (c *cwlogs) Delete(resource, name string) error {
	logGroup := fmt.Sprintf("%s%s-%s-%s", databasePrefix, c.config.Metadata.Name, c.namespace, resource)
	_, err := c.client.DeleteLogGroup(&cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroup),
	})
	return err
}

//readLogEvents tails the given stream names in the specified log group name
//To tail all the available streams logStreamName has to be '*'
//It returns a channel where logs line are published
//Unless the follow flag is true the channel is closed once there are no more events available
//
// The design is that a log group is created per custom resource definition, and a log stream is created custom resource.
func (c cwlogs) readLogEvents(logGroupName string, logStreamNamePrefix string, follow bool, startTime *time.Time) (<-chan *cloudwatchlogs.FilteredLogEvent, <-chan error) {
	cwl := c.client

	var lastSeenTimestamp *int64
	if startTime != nil {
		startTimeEpoch := startTime.Unix() * SecondInMillis
		lastSeenTimestamp = &startTimeEpoch
	} else {
		lastSeenTimestamp = nil
	}

	logEventsCh := make(chan *cloudwatchlogs.FilteredLogEvent)
	errCh := make(chan error)

	recentAlreadySeenLogEvents := &eventCache{seen: make(map[string]bool)}
	logStreams := &logStreams{}

	listUnseenLogStreams := func(logGroupName string, logStreamName string) ([]*string, error) {
		var streamNames []*string
		streamNamesCh, listErrCh := c.listLogStreams(logGroupName, logStreamName, lastSeenTimestamp)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		exiting := false
		for !exiting {
			select {
			case <-c:
				return nil, fmt.Errorf("interrupted")
			case err, ok := <-listErrCh:
				if ok {
					return nil, err
				}
				exiting = true
			case stream, ok := <-streamNamesCh:
				if ok {
					streamNames = append(streamNames, stream)
				} else {
					exiting = true
				}
			default:
				time.Sleep(100 * time.Millisecond)
			}
		}
		if len(streamNames) == 0 {
			return nil, fmt.Errorf("no such log stream(s).")
		}
		if len(streamNames) >= 100 { //FilterLogEventPages won't take more than 100 stream names
			streamNames = streamNames[0:100]
		}
		return streamNames, nil
	}

	logStreamRelistInterval := time.Second * 5

	//if *logStreamNamePrefix != "*" {
	//	ss, err := listUnseenLogStreams(logGroupName, logStreamNamePrefix)
	//	if err != nil {
	//		errCh <- err
	//		return logEventsCh, errCh
	//	}
	//	logStreams.reset(ss)
	//}

	pageHandler := func(res *cloudwatchlogs.FilterLogEventsOutput, lastPage bool) bool {
		for _, event := range res.Events {
			eventTimestamp := *event.Timestamp
			if lastSeenTimestamp == nil || eventTimestamp != *lastSeenTimestamp {
				lastSeenTimestamp = &eventTimestamp
				if recentAlreadySeenLogEvents.Size() >= 1000 {
					recentAlreadySeenLogEvents.Reset()
				}
			}

			if !recentAlreadySeenLogEvents.Has(*event.EventId) {
				recentAlreadySeenLogEvents.Add(*event.EventId)
				logEventsCh <- event
			} else {
				//fmt.Printf("%s already seen\n", *event.EventId)
			}
		}

		return !lastPage
	}

	go func() {
		defer close(logEventsCh)
		defer close(errCh)

		ss, err := listUnseenLogStreams(logGroupName, logStreamNamePrefix)
		if err != nil {
			errCh <- err
			return
		}
		logStreams.reset(ss)

		lastLogStreamsListTime := time.Now()
		for {
			if time.Now().After(lastLogStreamsListTime.Add(logStreamRelistInterval)) {
				lastLogStreamsListTime = time.Now()
				ss, err := listUnseenLogStreams(logGroupName, logStreamNamePrefix)
				if err != nil {
					errCh <- err
					return
				}
				logStreams.reset(ss)
			}
			//FilterLogEventPages won't take more than 100 stream names
			filter := createFilterLogEventsInput(logGroupName, logStreams.get(), lastSeenTimestamp)
			// Block until the last page is seen
			err := cwl.FilterLogEventsPages(filter, pageHandler)
			if err != nil {
				if awsErr, ok := err.(awserr.Error); ok {
					switch awsErr.Code() {
					case cloudwatchlogs.ErrCodeLimitExceededException, cloudwatchlogs.ErrCodeServiceUnavailableException:
						fmt.Fprintf(os.Stderr, "retrying on error: %v", err)
					default:
						errCh <- err
						return
					}
				}
			} else if !follow {
				return
			}
			//AWS API accepts 5 reqs/sec
			//time.Sleep(time.Millisecond * 205)
			time.Sleep(1 * time.Second)
		}
	}()

	return logEventsCh, errCh
}

func logStreamMatchesTimeRange(logStream *cloudwatchlogs.LogStream, startTimeMillis *int64) bool {
	if startTimeMillis == nil {
		return true
	}
	if logStream.CreationTime == nil || logStream.LastIngestionTime == nil {
		return false
	}
	lastIngestionAfterStartTime := startTimeMillis != nil && *logStream.LastIngestionTime >= *startTimeMillis-5*MinuteInMillis
	return lastIngestionAfterStartTime
}

// listLogStreams lists the streams of a given stream group
// It returns a channel where the stream names are published
func (c cwlogs) listLogStreams(groupName string, streamNamePrefix string, startTimeMillis *int64) (<-chan *string, <-chan error) {
	cwl := c.client
	streamNamesCh := make(chan *string)
	errCh := make(chan error)

	params := &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(groupName),
	}
	params.LogStreamNamePrefix = aws.String(streamNamePrefix)
	handler := func(res *cloudwatchlogs.DescribeLogStreamsOutput, lastPage bool) bool {
		for _, logStream := range res.LogStreams {
			if logStreamMatchesTimeRange(logStream, startTimeMillis) {
				fmt.Fprintf(os.Stderr, "fetched stream name: %s\n", *logStream.LogStreamName)
				streamNamesCh <- logStream.LogStreamName
			}
		}
		return !lastPage
	}

	go func() {
		defer close(streamNamesCh)
		defer close(errCh)

		for {
			err := cwl.DescribeLogStreamsPages(params, handler)

			if err == nil {
				fmt.Fprintf(os.Stderr, "finishing fetch\n")
				break
			}

			errCh <- err

			if awsErr, ok := err.(awserr.Error); ok {
				switch awsErr.Code() {
				case cloudwatchlogs.ErrCodeLimitExceededException, cloudwatchlogs.ErrCodeServiceUnavailableException:
					fmt.Fprintf(os.Stderr, "retrying in 1 second on error: %v\n", err)
					time.Sleep(100 * time.Millisecond)
				default:
					break
				}
			}
		}
	}()
	return streamNamesCh, errCh
}
