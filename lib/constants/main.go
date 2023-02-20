package constants

const (
	CommonFinalizer     string = "finalizers.kloudlite.io"
	ForegroundFinalizer string = "foregroundDeletion"
	KlFinalizer         string = "klouldite-finalizer"
)

const (
	MainNs string = "kl-core"
)

const (
	ClearStatusKey string = "kloudlite.io/clear-status"
	RestartKey     string = "kloudlite.io/do-restart"
)

const (
	AccountNameKey string = "kloudlite.io/account.name"
	EdgeNameKey    string = "kloudlite.io/edge.name"
	ClusterNameKey string = "kloudlite.io/cluster.name"

	ProjectNameKey       string = "kloudlite.io/project.name"
	MsvcNameKey          string = "kloudlite.io/msvc.name"
	MresNameKey          string = "kloudlite.io/mres.name"
	AppNameKey           string = "kloudlite.io/app.name"
	RouterNameKey        string = "kloudlite.io/router.name"
	LambdaNameKey        string = "kloudlite.io/lambda.name"
	AccountRouterNameKey string = "kloudlite.io/account-router.name"
	// DeviceWgKey          string = "klouldite.io/wg-device-key"
	DeviceWgKey string = "kloudlite.io/is-wg-key"
	WgService   string = "kloudlite.io/wg-service"
	WgDeploy    string = "kloudlite.io/wg-deployment"
	WgDomain    string = "kloudlite.io/wg-domain"

	ProviderNameKey string = "kloudlite.io/provider.name"

	NodePoolKey string = "kloudlite.io/node-pool"
	RegionKey   string = "kloudlite.io/region"
	NodeIndex   string = "kloudlite.io/node-index"
	NodeIps     string = "kloudlite.io/node-ips"

	GroupVersionKind string = "kloudlite.io/group-version-kind"
)

const (
	RegionKind string = "regions.wg.kloudlite.io"
)
