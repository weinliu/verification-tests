package mco

import (
	"fmt"
	"sort"
	"time"

	"github.com/onsi/gomega/types"

	g "github.com/onsi/ginkgo/v2"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

// Event struct is used to handle Event resources in OCP
type Event struct {
	Resource
}

// EventList handles list of nodes
type EventList struct {
	ResourceList
}

// NewEvent create a Event struct
func NewEvent(oc *exutil.CLI, namespace, name string) *Event {
	return &Event{Resource: *NewNamespacedResource(oc, "Event", namespace, name)}
}

// String implements the Stringer interface
func (e Event) String() string {
	e.oc.NotShowInfo()
	defer e.oc.SetShowInfo()

	description, err := e.Get(`{.metadata.creationTimestamp} Type: {.type} Reason: {.reason} Namespace: {.metadata.namespace} Involves: {.involvedObject.kind}/{.involvedObject.name}`)
	if err != nil {
		logger.Errorf("Event %s/%s does not exist anymore", e.GetNamespace(), e.GetName())
		return ""
	}

	return description
}

// NewEventList construct a new node list struct to handle all existing nodes
func NewEventList(oc *exutil.CLI, namespace string) *EventList {
	return &EventList{*NewNamespacedResourceList(oc, "Event", namespace)}
}

// GetAll returns a []Event list with all existing events sorted by creation time
// the first element will be the most recent one
func (el *EventList) GetAll() ([]Event, error) {
	el.ResourceList.SortByTimestamp()
	allEventResources, err := el.ResourceList.GetAll()
	if err != nil {
		return nil, err
	}
	allEvents := make([]Event, 0, len(allEventResources))

	for _, eventRes := range allEventResources {
		allEvents = append(allEvents, *NewEvent(el.oc, eventRes.namespace, eventRes.name))
	}
	// We want the first element to be the more recent
	allEvents = reverseEventsList(allEvents)

	return allEvents, nil
}

// GetLatest returns the latest event that occurred. Nil if no event exists.
func (el *EventList) GetLatest() (*Event, error) {

	allEvents, lerr := el.GetAll()
	if lerr != nil {
		logger.Errorf("Error getting events %s", lerr)
		return nil, lerr
	}
	if len(allEvents) == 0 {
		return nil, nil
	}

	return &(allEvents[0]), nil
}

// GetAllEventsSinceEvent returns all events that occurred since a given event (not included)
func (el *EventList) GetAllEventsSinceEvent(sinceEvent *Event) ([]Event, error) {
	allEvents, lerr := el.GetAll()
	if lerr != nil {
		logger.Errorf("Error getting events %s", lerr)
		return nil, lerr
	}

	if sinceEvent == nil {
		return allEvents, nil
	}

	returnEvents := []Event{}
	for _, event := range allEvents {
		if event.name == sinceEvent.name {
			break
		}
		returnEvents = append(returnEvents, event)
	}

	return returnEvents, nil
}

// GetAllSince return a list of the events that happened since the provided duration
func (el EventList) GetAllSince(since time.Time) ([]Event, error) {

	allEvents, lerr := el.GetAll()
	if lerr != nil {
		logger.Errorf("Error getting events %s", lerr)
		return nil, lerr
	}

	returnEvents := []Event{}
	for _, event := range allEvents {
		creationTime, err := event.Get(`{.metadata.creationTimestamp}`)
		if err != nil {
			logger.Errorf("Error parsing event %s/%s. Error: %s", event.GetNamespace(), event.GetName(), err)
			continue
		}

		parsedCreation, perr := time.Parse(time.RFC3339, creationTime)
		if perr != nil {
			logger.Errorf("Error parsing event '%s' -n '%s' creation time: %s", event.GetName(), event.GetNamespace(), perr)
			return nil, perr

		}
		if parsedCreation.After(since) {
			returnEvents = append(returnEvents, event)
		}
	}

	return returnEvents, nil
}

// from https://github.com/golang/go/wiki/SliceTricks#reversing
func reverseEventsList(a []Event) []Event {

	for i := len(a)/2 - 1; i >= 0; i-- {
		opp := len(a) - 1 - i
		a[i], a[opp] = a[opp], a[i]
	}

	return a
}

// HaveEventsSequence returns a gomega matcher that checks if a list of Events contains a given sequence of reasons
func HaveEventsSequence(sequence ...string) types.GomegaMatcher {
	return &haveEventsSequenceMatcher{sequence: sequence}
}

// struct to cache and sort events information
type tmpEvent struct {
	creationTimestamp time.Time
	reason            string
}

func (t tmpEvent) String() string { return fmt.Sprintf("%s - %s", t.creationTimestamp, t.reason) }

// sorter to sort the chache event list
type byCreationTime []tmpEvent

func (a byCreationTime) Len() int      { return len(a) }
func (a byCreationTime) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byCreationTime) Less(i, j int) bool {
	return a[i].creationTimestamp.Before(a[j].creationTimestamp)
}

// struct implementing gomaega matcher interface
type haveEventsSequenceMatcher struct {
	sequence []string
}

func (matcher *haveEventsSequenceMatcher) Match(actual interface{}) (success bool, err error) {

	logger.Infof("Start verifying events sequence: %s", matcher.sequence)
	events, ok := actual.([]Event)
	if !ok {
		return false, fmt.Errorf("HaveSequence matcher expects a slice of Events in test case %v", g.CurrentSpecReport().FullText())
	}

	// To avoid too many "oc" executions we store the events information in a cached struct list with "creationTimestamp" and "reason" fields.
	tmpEvents := []tmpEvent{}
	for _, loopEvent := range events {
		event := loopEvent // this is to make sure that we execute defer in all events, and not only in the last one

		event.oc.NotShowInfo()
		defer event.oc.SetShowInfo()

		reason, err := event.Get(`{.reason}`)
		if err != nil {
			return false, err
		}
		creationStamp, cerr := event.Get(`{.metadata.creationTimestamp}`)
		if cerr != nil {
			return false, cerr
		}
		creation, perr := time.Parse(time.RFC3339, creationStamp)
		if perr != nil {
			return false, err
		}
		tmpEvents = append(tmpEvents, tmpEvent{creationTimestamp: creation, reason: reason})
	}

	// We sort the cached list. Oldest event first
	sort.Sort(byCreationTime(tmpEvents))

	// Several events can be created in the same second, hence, we need to take into account
	// that 2 events in the same second can match any order.
	// If 2 events have the same timestamp
	// we consider that the order is right no matter what.
	lastEventTime := time.Time{}
	for _, seqReason := range matcher.sequence {
		found := false
		for _, event := range tmpEvents {
			if seqReason == event.reason &&
				(lastEventTime.Before(event.creationTimestamp) || lastEventTime.Equal(event.creationTimestamp)) {
				logger.Infof("Found! %s event in time %s", seqReason, event.creationTimestamp)

				lastEventTime = event.creationTimestamp
				found = true
				break
			}
		}

		// Could not find an event with the sequence's reason. We fail the match
		if !found {
			logger.Errorf("%s event NOT Found after time %s", seqReason, lastEventTime)
			return false, nil
		}
	}

	return true, nil
}

func (matcher *haveEventsSequenceMatcher) FailureMessage(actual interface{}) (message string) {
	// The type was already validated in Match, we can safely ignore the error
	events, _ := actual.([]Event)

	output := "Expecte events\n"

	if len(events) == 0 {
		output = "No events in the list\n"
	} else {
		for _, event := range events {
			output += fmt.Sprintf("-  %s\n", event)
		}
	}
	output += fmt.Sprintf("to contain this reason sequence\n\t%s\n", matcher.sequence)

	return output
}

func (matcher *haveEventsSequenceMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	// The type was already validated in Match, we can safely ignore the error
	events, _ := actual.([]Event)

	output := "Expecte events\n"
	for _, event := range events {
		output += output + fmt.Sprintf("-  %s\n", event)
	}
	output += output + fmt.Sprintf("NOT to contain this reason sequence\n\t%s\n", matcher.sequence)

	return output
}
