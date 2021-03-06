package discovery

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/coreos/go-etcd/etcd"
	"github.com/stretchr/testify/require"
)

func TestMockClient(t *testing.T) {
	t.Parallel()
	runTest(t, NewMockClient())
}

func TestEtcdClient(t *testing.T) {
	t.Parallel()
	client, err := getEtcdClient()
	require.NoError(t, err)
	runTest(t, client)
}

func TestEtcdWatch(t *testing.T) {
	t.Parallel()
	client, err := getEtcdClient()
	require.NoError(t, err)
	runWatchTest(t, client)
}

func runTest(t *testing.T, client Client) {
	require.NoError(t, client.Set("foo", "one", 0))
	value, ok, err := client.Get("foo")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "one", value)
	//values, err := client.GetAll("foo")
	//require.NoError(t, err)
	//require.Equal(t, map[string]string{"foo": "one"}, values)

	require.NoError(t, client.Set("a/b/foo", "one", 0))
	require.NoError(t, client.Set("a/b/bar", "two", 0))
	values, err := client.GetAll("a/b")
	require.NoError(t, err)
	require.Equal(t, map[string]string{"a/b/foo": "one", "a/b/bar": "two"}, values)

	require.NoError(t, client.Close())
}

func runWatchTest(t *testing.T, client Client) {
	cancel := make(chan bool)
	err := client.Watch(
		"watch/foo",
		cancel,
		func(value string) error {
			if value == "" {
				require.NoError(t, client.Set("watch/foo", "bar", 0))
			} else {
				require.Equal(t, "bar", value)
				close(cancel)
			}
			return nil
		},
	)
	require.Equal(t, etcd.ErrWatchStoppedByUser, err)

	cancel = make(chan bool)
	err = client.WatchAll(
		"watchAll/foo",
		cancel,
		func(value map[string]string) error {
			if value == nil {
				require.NoError(t, client.Set("watchAll/foo/bar", "quux", 0))
			} else {
				require.Equal(t, map[string]string{"watchAll/foo/bar": "quux"}, value)
				close(cancel)
			}
			return nil
		},
	)
	require.Equal(t, etcd.ErrWatchStoppedByUser, err)
}

func getEtcdClient() (Client, error) {
	etcdAddress, err := getEtcdAddress()
	if err != nil {
		return nil, err
	}
	return NewEtcdClient(etcdAddress), nil
}

func getEtcdAddress() (string, error) {
	etcdAddr := os.Getenv("ETCD_PORT_2379_TCP_ADDR")
	if etcdAddr == "" {
		return "", errors.New("ETCD_PORT_2379_TCP_ADDR not set")
	}
	return fmt.Sprintf("http://%s:2379", etcdAddr), nil
}
