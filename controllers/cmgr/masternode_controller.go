package cmgr

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"

	"gopkg.in/yaml.v2"

	"time"

	"github.com/hashicorp/go-uuid"
	cmgrv1 "github.com/kloudlite/cluster-operator/apis/cmgr/v1"

	"github.com/kloudlite/cluster-operator/env"

	"github.com/kloudlite/cluster-operator/lib/constants"
	fn "github.com/kloudlite/cluster-operator/lib/functions"
	"github.com/kloudlite/cluster-operator/lib/kubectl"
	"github.com/kloudlite/cluster-operator/lib/logging"
	nodejobcrgen "github.com/kloudlite/cluster-operator/lib/nodejob-cr-generator"
	rApi "github.com/kloudlite/cluster-operator/lib/operator"
	stepResult "github.com/kloudlite/cluster-operator/lib/operator/step-result"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/kloudlite/cluster-operator/lib/terraform"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MasterNodeReconciler reconciles a MasterNode object
type MasterNodeReconciler struct {
	Env *env.Env
	client.Client
	Scheme *runtime.Scheme
	logger logging.Logger
	Name   string

	yamlClient *kubectl.YAMLClient
}

func (r *MasterNodeReconciler) GetName() string {
	return r.Name
}

const (
	MasterNodeReady string = "master-node-ready"
	NodeCreated     string = "node-created"
	K3SInstalled    string = "k3s-installed"
)

//+kubebuilder:rbac:groups=cmgr.kloudlite.io,resources=masternodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cmgr.kloudlite.io,resources=masternodes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cmgr.kloudlite.io,resources=masternodes/finalizers,verbs=update

/*
# actions needs to be performed
1. check if node created
2. if not created create
3. check if k3s installed
4. if not installed install
5. if deletion timestamp present
	- delete node from the cluster
	- delete the actual master node
*/

func (r *MasterNodeReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	req, err := rApi.NewRequest(rApi.NewReconcilerCtx(ctx, r.logger), r.Client, request.NamespacedName, &cmgrv1.MasterNode{})
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if step := req.EnsureChecks(MasterNodeReady, NodeCreated, K3SInstalled, SSHReady); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if req.Object.GetDeletionTimestamp() != nil {
		if x := r.finalize(req); !x.ShouldProceed() {
			return x.ReconcilerResponse()
		}
		return ctrl.Result{}, nil
	}

	req.Logger.Infof("NEW RECONCILATION")

	if step := req.ClearStatusIfAnnotated(); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := req.RestartIfAnnotated(); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := req.EnsureLabelsAndAnnotations(); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := req.EnsureFinalizers(constants.ForegroundFinalizer, constants.CommonFinalizer); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.EnsureSSH(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.EnsureNodeCreated(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	if step := r.EnsureK3SInstalled(req); !step.ShouldProceed() {
		return step.ReconcilerResponse()
	}

	req.Object.Status.IsReady = true
	req.Logger.Infof("RECONCILATION COMPLETE")
	return ctrl.Result{RequeueAfter: ReconcilationPeriod * time.Second}, r.Status().Update(ctx, req.Object)
}

func (r *MasterNodeReconciler) EnsureSSH(req *rApi.Request[*cmgrv1.MasterNode]) stepResult.Result {

	ctx, obj, checks := req.Context(), req.Object, req.Object.Status.Checks
	check := rApi.Check{Generation: obj.Generation}
	failed := func(err error) stepResult.Result {
		return req.CheckFailed(SSHReady, check, err.Error())
	}

	secName := fmt.Sprintf("ssh-cluster-%s", obj.Spec.ClusterName)

	sec, err := rApi.Get(
		ctx, r.Client, types.NamespacedName{
			Name:      secName,
			Namespace: constants.MainNs, // TODO
		},
		&corev1.Secret{},
	)

	if err != nil {
		return failed(err)
	}

	rApi.SetLocal(req, "ssh-sec", sec)

	check.Status = true
	if check != checks[SSHReady] {
		checks[SSHReady] = check
		req.UpdateStatus()
	}

	return req.Next()
}

func (r *MasterNodeReconciler) EnsureNodeCreated(req *rApi.Request[*cmgrv1.MasterNode]) stepResult.Result {

	ctx, obj, checks := req.Context(), req.Object, req.Object.Status.Checks
	check := rApi.Check{Generation: obj.Generation}
	failed := func(err error) stepResult.Result {
		return req.CheckFailed(NodeCreated, check, err.Error())
	}

	_, err := terraform.GetOutputs(r.getTFPath(obj))
	if err != nil {
		// node is created // needs to check its status
		if err := r.createNode(req); err != nil {
			return failed(err)
		}

		return failed(fmt.Errorf("node scheduled to create"))
	}

	_, err = rApi.Get(
		ctx, r.Client, types.NamespacedName{
			Name:      fmt.Sprintf("create-node-%s", obj.Name),
			Namespace: constants.MainNs,
		},
		&batchv1.Job{},
	)

	if err == nil {
		if err := r.Client.Delete(ctx, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("create-node-%s", obj.Name),
				Namespace: constants.MainNs,
			},
		}); err != nil {
			return failed(err)
		}
	}

	check.Status = true
	if check != checks[NodeCreated] {
		checks[NodeCreated] = check
		return req.UpdateStatus()
	}
	return req.Next()
}

func (r *MasterNodeReconciler) syncKubeConfig(req *rApi.Request[*cmgrv1.MasterNode], ip string, update bool) error {
	ctx, obj := req.Context(), req.Object

	sshSec, ok := rApi.GetLocal[*corev1.Secret](req, "ssh-sec")
	if !ok {
		return fmt.Errorf("no ssh sec found")
	}

	access, ok := sshSec.Data["access"]
	if !ok {
		return fmt.Errorf("access not available in sec")
	}

	filename, err := uuid.GenerateUUID()
	if err != nil {
		return err
	}

	s := os.TempDir()
	fname := fmt.Sprintf("%s/%s", s, filename)
	if err := os.WriteFile(fname, access, 0400); err != nil {
		return err
	}
	defer os.Remove(fname)

	out, err := fn.ExecCmd(fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i %s root@%s cat /etc/rancher/k3s/k3s.yaml", fname, ip), "", true)
	if err != nil {
		return err
	}

	var kubeconfig KubeConfigType
	if err := yaml.Unmarshal(out, &kubeconfig); err != nil {
		return err
	}

	for i := range kubeconfig.Clusters {
		kubeconfig.Clusters[i].Cluster.Server = fmt.Sprintf("https://%s:6443", ip)
	}

	out, err = yaml.Marshal(kubeconfig)
	if err != nil {
		return err
	}

	tokenOut, err := fn.ExecCmd(fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i %s root@%s cat /var/lib/rancher/k3s/server/node-token", fname, ip), "", true)
	if err != nil {
		return err
	}

	if update {

		err = r.Client.Update(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("cluster-kubeconfig-%s", obj.Spec.ClusterName),
				Namespace: constants.MainNs,
			},
			Data: map[string][]byte{
				"kubeconfig":     out,
				"node-token":     tokenOut,
				"master-node-ip": []byte(ip),
			},
		})

	} else {
		err = r.Client.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("cluster-kubeconfig-%s", obj.Spec.ClusterName),
				Namespace: constants.MainNs,
			},
			Data: map[string][]byte{
				"kubeconfig":     out,
				"node-token":     tokenOut,
				"master-node-ip": []byte(ip),
			},
		})
	}

	return err
}

func (r *MasterNodeReconciler) EnsureK3SInstalled(req *rApi.Request[*cmgrv1.MasterNode]) stepResult.Result {
	// ping to 6443 port if return err code means not installed or not running

	ctx, obj, checks := req.Context(), req.Object, req.Object.Status.Checks
	check := rApi.Check{Generation: obj.Generation}

	failed := func(err error) stepResult.Result {
		r.logger.Errorf(err, obj.Name)
		return req.CheckFailed(K3SInstalled, check, err.Error())
	}

	ip, err := terraform.GetOutput(r.getTFPath(obj), "node-ip")
	if err != nil {
		// may be not created needs to create throw error so will be reconcile aftter some time
		return failed(err)
	}

	if _, err := http.Get(fmt.Sprintf("http://%s:6443", ip)); err != nil {
		if ee := r.installMasterOnNode(req, ip); ee != nil {
			return failed(err)
		}
		return failed(err)
	}

	kubeConfigSec, err := rApi.Get(ctx, r.Client, types.NamespacedName{
		Name:      fmt.Sprintf("cluster-kubeconfig-%s", obj.Spec.ClusterName),
		Namespace: constants.MainNs,
	}, &corev1.Secret{})

	if err != nil {
		if !apiErrors.IsNotFound(err) {
			return failed(err)
		}
		if e := r.syncKubeConfig(req, ip, false); e != nil {
			return failed(e)
		}
		return failed(err)
	}

	if _, err = fn.KubectlWithConfig("get nodes -ojson", kubeConfigSec.Data["kubeconfig"]); err != nil {
		if e := r.syncKubeConfig(req, ip, true); e != nil {
			return failed(e)
		}
		return failed(err)
	}

	check.Status = true
	if check != checks[K3SInstalled] {
		checks[K3SInstalled] = check
		return req.UpdateStatus()
	}
	return req.Next()
}

func (r *MasterNodeReconciler) finalize(req *rApi.Request[*cmgrv1.MasterNode]) stepResult.Result {
	// NOTE: for now ignore deletion of ndoe from the cluster as there will be only one masternode in dev mode

	ctx, obj := req.Context(), req.Object
	/*
		Steps to finalize
		1. check if node deleted
			1. if deleted finalize
			2. if not deleted
				1. check if deletion job is running
					1. if deletion job is finished continue on next reconcile
					2. if running wait for the finish
					3. if not running delete the creation job if any running and then create deletion job
	*/

	// if creation job is running delete it
	if _, err := rApi.Get(
		ctx, r.Client, types.NamespacedName{
			Name:      fmt.Sprintf("create-node-%s", obj.Name),
			Namespace: constants.MainNs,
		},
		&batchv1.Job{},
	); err == nil {
		if err := r.Client.Delete(ctx, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("create-node-%s", obj.Name),
				Namespace: constants.MainNs,
			},
		}); err != nil {
			return req.FailWithStatusError(err)
		}
	}

	// if node is deleted wait for the deletion job to be finished
	if _, err := terraform.GetOutputs(r.getTFPath(obj)); err != nil {
		// node not present wait for the job finish and finalize
		if deletionJob, err := rApi.Get(
			ctx, r.Client, types.NamespacedName{
				Name:      fmt.Sprintf("delete-node-%s", obj.Name),
				Namespace: constants.MainNs,
			},
			&batchv1.Job{},
		); err == nil {
			if deletionJob.Status.Active > 0 {
				return req.FailWithStatusError(fmt.Errorf("waiting for deletion job to be finished"))
			}

			if err := r.Client.Delete(ctx, &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("delete-node-%s", obj.Name),
					Namespace: constants.MainNs,
				},
			}); err != nil {
				return req.FailWithStatusError(err)
			}
		}

		return req.Finalize()
	} else {
		// node present create deletion job

		// check if job created if created wait for the completion
		if _, err = rApi.Get(
			ctx, r.Client, types.NamespacedName{
				Name:      fmt.Sprintf("delete-node-%s", obj.Name),
				Namespace: constants.MainNs,
			},
			&batchv1.Job{},
		); err == nil {
			return req.FailWithStatusError(fmt.Errorf("deletion in progress"))
		}

		if err := r.deleteNode(req); err != nil {
			return req.FailWithStatusError(err)
		}
		return req.FailWithStatusError(fmt.Errorf("deletion in progress"))
	}
}

func mNode(name string) string {
	return fmt.Sprintf("kl-byoc-master-%s", name)
}

func (r *MasterNodeReconciler) getTFPath(obj *cmgrv1.MasterNode) string {
	// eg -> /path/acc_id/do/blr1/node_id/do
	// eg -> /path/acc_id/aws/ap-south-1/node_id/aws
	tfPath := path.Join(r.Env.StorePath, obj.Spec.AccountName, obj.Spec.Provider, obj.Spec.Region, mNode(obj.Name), obj.Spec.Provider)
	r.logger.Debugf(tfPath)
	return tfPath
}

func (r *MasterNodeReconciler) createNode(req *rApi.Request[*cmgrv1.MasterNode]) error {

	ctx, obj := req.Context(), req.Object

	_, err := rApi.Get(
		ctx, r.Client, types.NamespacedName{
			Name:      fmt.Sprintf("create-node-%s", obj.Name),
			Namespace: constants.MainNs,
		},
		&batchv1.Job{},
	)

	if err == nil {
		return fmt.Errorf("creation of node is in progress")
	}

	// jobOut, err := r.getJobCrd(req, true)
	// if err != nil {
	// 	return err
	// }

	jobOut, err := nodejobcrgen.GetJobCrd(ctx, r.Client, nodejobcrgen.JobCrdSpecs{
		Name:               obj.Name,
		ProviderName:       obj.Spec.ProviderName,
		JobStorePath:       r.Env.JobStorePath,
		JobTFTemplatesPath: r.Env.JobTFTemplatesPath,
		JobSSHPath:         r.Env.JobSSHPath,
		Provider:           obj.Spec.Provider,
		Config:             obj.Spec.Config,
		AccountName:        obj.Spec.AccountName,
		Region:             obj.Spec.Region,
		Owners:             []metav1.OwnerReference{fn.AsOwner(obj, true)},
		ClusterName:        obj.Spec.ClusterName,
		NodeName:           mNode(obj.Name),
	}, true)
	if err != nil {
		return err
	}

	// fmt.Println(string(jobOut_))

	if err = r.yamlClient.ApplyYAML(ctx, jobOut); err != nil {
		return err
	}

	return nil
}

func (r *MasterNodeReconciler) deleteNode(req *rApi.Request[*cmgrv1.MasterNode]) error {
	/*
		1. check if deletion job is already present
		- if present return with deletion in progress
		- else create deletion Job
	*/
	// if job not created and node deleted then create job

	// needs to create deletionJob
	// jobOut, err := r.getJobCrd(req, false)
	// if err != nil {
	// 	return err
	// }
	ctx, obj := req.Context(), req.Object

	jobOut, err := nodejobcrgen.GetJobCrd(ctx, r.Client, nodejobcrgen.JobCrdSpecs{
		Name:               obj.Name,
		ProviderName:       obj.Spec.ProviderName,
		JobStorePath:       r.Env.JobStorePath,
		JobTFTemplatesPath: r.Env.JobTFTemplatesPath,
		JobSSHPath:         r.Env.JobSSHPath,
		Provider:           obj.Spec.Provider,
		Config:             obj.Spec.Config,
		AccountName:        obj.Spec.AccountName,
		Region:             obj.Spec.Region,
		Owners:             []metav1.OwnerReference{fn.AsOwner(obj, true)},
		ClusterName:        obj.Spec.ClusterName,
		NodeName:           mNode(obj.Name),
	}, false)
	if err != nil {
		return err
	}

	if err := r.yamlClient.ApplyYAML(ctx, jobOut); err != nil {
		return err
	}

	r.logger.Debugf("node scheduled to delete")
	return nil
}

func (r *MasterNodeReconciler) installMasterOnNode(req *rApi.Request[*cmgrv1.MasterNode], ip string) error {
	// cmd := fmt.Sprintf(
	// 	"ssh  -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i %s root@%s sudo sh /tmp/k3s-install.sh server --token=%q  --datastore-endpoint=%q --node-external-ip %s --flannel-backend wireguard-native --flannel-external-ip --disable traefik",
	// 	r.Env.SSHPath,
	// 	ip,
	// 	obj.Name,
	// 	obj.Spec.MysqlURI,
	// 	ip,
	// )

	ctx, obj := req.Context(), req.Object
	dbName := fmt.Sprintf("cluster-%s", obj.Spec.ClusterName)

	mysqlSecret, err := rApi.Get(
		ctx, r.Client, types.NamespacedName{
			Name:      fmt.Sprintf("mres-%s", dbName),
			Namespace: constants.MainNs, // TODO
		},
		&corev1.Secret{},
	)

	if err != nil {
		return err
	}
	// // we got uri for the node creation
	uri, ok := mysqlSecret.Data["EXTERNAL_DSN"]

	if !ok {
		return fmt.Errorf("can't get dsn of db")
	}

	sshSec, ok := rApi.GetLocal[*corev1.Secret](req, "ssh-sec")
	if !ok {
		return fmt.Errorf("no ssh sec found")
	}

	access, ok := sshSec.Data["access"]
	if !ok {
		return fmt.Errorf("access not available in sec")
	}

	filename, err := uuid.GenerateUUID()
	if err != nil {
		return err
	}

	s := os.TempDir()
	fname := fmt.Sprintf("%s/%s", s, filename)
	if err := os.WriteFile(fname, access, 0400); err != nil {
		return err
	}
	defer os.Remove(fname)

	cmd := fmt.Sprintf(
		"ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i %s root@%s sudo sh /tmp/k3s-install.sh server --token=%q  --datastore-endpoint=%q --node-external-ip %s --flannel-backend wireguard-native --flannel-external-ip --disable traefik --node-name=%q",
		fname,
		ip,
		obj.Name,
		uri,
		ip,
		mNode(obj.Name),
	)

	if _, err = fn.ExecCmd(cmd, "", false); err != nil {
		return err
	}

	return fmt.Errorf("installation in progress")
}

func (r *MasterNodeReconciler) removeMasterFromCluster(obj *cmgrv1.MasterNode) error {
	// TODO: have to delete from the node
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MasterNodeReconciler) SetupWithManager(mgr ctrl.Manager, logger logging.Logger) error {
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	r.logger = logger.WithName(r.Name)
	r.yamlClient = kubectl.NewYAMLClientOrDie(mgr.GetConfig())

	return ctrl.NewControllerManagedBy(mgr).
		For(&cmgrv1.MasterNode{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
