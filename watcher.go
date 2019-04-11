package dockeripv6nat

import (
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsouza/go-dockerclient"
)

type RecoverableError struct {
	err error
}

func (re *RecoverableError) Error() string {
	return re.err.Error()
}

// Number of seconds to wait after connection failure.
const retryInterval = 10

type watcher struct {
	client        *docker.Client
	state         *state
	eventChannel  chan *docker.APIEvents
	signalChannel chan os.Signal
	retry         bool
}

func NewWatcher(client *docker.Client, state *state, retry bool) *watcher {
	return &watcher{
		client: client,
		state:  state,
		retry:  retry,
	}
}

func (w *watcher) Watch() error {
	w.signalChannel = make(chan os.Signal, 1)
	signal.Notify(w.signalChannel, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGKILL)
	defer signal.Stop(w.signalChannel)

	done := false
	for !done {
		if w.eventChannel == nil {
			if err := w.attemptRecovery(w.setupListener()); err != nil {
				return err
			}
		}

		var err error
		done, err = w.processOnce()
		if err := w.attemptRecovery(err); err != nil {
			return err
		}
	}

	return nil
}

func (w *watcher) attemptRecovery(err error) error {
	if err == nil {
		return nil
	}

	if errRecoverable, match := err.(*RecoverableError); match && w.retry {
		if w.eventChannel != nil {
			w.client.RemoveEventListener(w.eventChannel)
			w.eventChannel = nil
		}
		log.Printf("%v", errRecoverable.err)
		return nil
	}

	return err
}

func (w *watcher) setupListener() error {
	// Always try a ping first
	if err := w.client.Ping(); err != nil {
		return &RecoverableError{err}
	}

	w.eventChannel = make(chan *docker.APIEvents, 1024)
	if err := w.client.AddEventListener(w.eventChannel); err != nil {
		return &RecoverableError{err}
	}

	if err := w.regenerate(); err != nil {
		return err
	}

	return nil
}

func (w *watcher) processOnce() (bool, error) {
	select {
	case <-time.After(retryInterval * time.Second):
		if w.eventChannel != nil {
			if err := w.client.Ping(); err != nil {
				return false, &RecoverableError{err}
			}
		}
	case event, ok := <-w.eventChannel:
		if !ok {
			return false, &RecoverableError{errors.New("docker daemon connection interrupted")}
		}
		if err := w.handleEvent(event); err != nil {
			// Wrap in a RecoverableError so that a regenerate will be initiated.
			return false, &RecoverableError{err}
		}
	case sig := <-w.signalChannel:
		if sig == syscall.SIGHUP {
			// Return a RecoverableError so that a regenerate will be initiated.
			return false, &RecoverableError{errors.New("received SIGHUP")}
		} else {
			return true, nil
		}
	}

	return false, nil
}

func (w *watcher) regenerate() error {
	networks, err := w.client.ListNetworks()
	if err != nil {
		return &RecoverableError{err}
	}

	networkIDs := make([]string, len(networks))
	for index, network := range networks {
		networkIDs[index] = network.ID
		if err := w.state.UpdateNetwork(network.ID, &network); err != nil {
			return err
		}
	}

	apiContainers, err := w.client.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		return &RecoverableError{err}
	}

	containerIDs := make([]string, len(apiContainers))
	for index, apiContainer := range apiContainers {
		containerIDs[index] = apiContainer.ID
		container, err := w.client.InspectContainer(apiContainer.ID)
		if err != nil {
			if _, match := err.(*docker.NoSuchContainer); match {
				container = nil
			} else {
				return &RecoverableError{err}
			}
		}
		if err := w.state.UpdateContainer(apiContainer.ID, container); err != nil {
			return err
		}
	}

	if err := w.state.RemoveMissingContainers(containerIDs); err != nil {
		return err
	}

	if err := w.state.RemoveMissingNetworks(networkIDs); err != nil {
		return err
	}

	return nil
}

func (w *watcher) handleEvent(event *docker.APIEvents) error {
	if event.Type != "network" {
		return nil
	}

	networkID := event.Actor.ID

	switch event.Action {
	case "create":
		network, err := w.client.NetworkInfo(networkID)
		if err != nil {
			if _, match := err.(*docker.NoSuchNetwork); match {
				network = nil
			} else {
				return &RecoverableError{err}
			}
		}
		if err := w.state.UpdateNetwork(networkID, network); err != nil {
			return err
		}
	case "destroy":
		if err := w.state.UpdateNetwork(networkID, nil); err != nil {
			return err
		}
	case "connect", "disconnect":
		containerID := event.Actor.Attributes["container"]
		container, err := w.client.InspectContainer(containerID)
		if err != nil {
			if _, match := err.(*docker.NoSuchContainer); match {
				container = nil
			} else {
				return &RecoverableError{err}
			}
		}
		if err := w.state.UpdateContainer(containerID, container); err != nil {
			return err
		}
	}

	return nil
}
