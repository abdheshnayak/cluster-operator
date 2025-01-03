package functions

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hashicorp/go-uuid"
	"github.com/kloudlite/cluster-operator/lib/errors"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewBool(b bool) *bool {
	return &b
}

func KubectlApplyExec(stdin ...[]byte) (stdout *bytes.Buffer, err error) {
	c := exec.Command("kubectl", "apply", "-f", "-")
	outStream, errStream := bytes.NewBuffer([]byte{}), bytes.NewBuffer([]byte{})
	c.Stdin = bytes.NewBuffer(bytes.Join(stdin, []byte("\n---\n")))
	c.Stdout = outStream
	c.Stderr = errStream
	if err := c.Run(); err != nil {
		return outStream, errors.NewEf(err, errStream.String())
	}
	fmt.Printf("stdout: %s\n", outStream.Bytes())
	return outStream, nil
}

func ExecCmd(cmdString string, logStr string, withCsv bool) ([]byte, error) {
	var cmdArr []string
	var err error
	if withCsv {

		r := csv.NewReader(strings.NewReader(cmdString))
		r.Comma = ' '
		cmdArr, err = r.Read()

		if err != nil {
			return nil, err
		}
	} else {
		cmdArr = strings.Split(cmdString, " ")
	}

	if logStr != "" {
		fmt.Printf("[#] %s\n", logStr)
	} else {
		fmt.Printf("[#] %s\n", strings.Join(cmdArr, " "))
	}

	cmd := exec.Command(cmdArr[0], cmdArr[1:]...)
	cmd.Stderr = os.Stderr
	// cmd.Stdout = os.Stdout

	if out, err := cmd.Output(); err != nil {
		fmt.Printf("err occurred: %s\n", err.Error())
		return nil, err
	} else {
		return out, err
	}
}

func Kubectl(args ...string) (stdout *bytes.Buffer, err error) {
	c := exec.Command("kubectl", args...)
	outStream, errStream := bytes.NewBuffer([]byte{}), bytes.NewBuffer([]byte{})
	c.Stdout = outStream
	c.Stderr = errStream
	if err := c.Run(); err != nil {
		return outStream, errors.NewEf(err, errStream.String())
	}
	// fmt.Printf("stdout: %s\n", outStream.Bytes())
	return outStream, nil
}

func toUnstructured(obj client.Object) (*unstructured.Unstructured, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}

	t := &unstructured.Unstructured{Object: m}
	return t, nil
}

func KubectlApply(ctx context.Context, cli client.Client, obj client.Object) error {
	t, err := toUnstructured(obj)
	if err != nil {
		return err
	}
	if err := cli.Get(
		ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, t,
	); err != nil {
		if !apiErrors.IsNotFound(err) {
			return errors.NewEf(err, "could not get k8s resource")
		}
		// CREATE it
		return cli.Create(ctx, obj)
	}

	// UPDATE it
	x, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	var j map[string]interface{}
	if err := json.Unmarshal(x, &j); err != nil {
		return err
	}

	if _, ok := j["spec"]; ok {
		t.Object["spec"] = j["spec"]
	}

	if _, ok := j["data"]; ok {
		t.Object["data"] = j["data"]
	}

	if _, ok := j["stringData"]; ok {
		t.Object["stringData"] = j["stringData"]
	}
	return cli.Update(ctx, t)
}

func KubectlGet(namespace string, resourceRef string) ([]byte, error) {
	c := exec.Command("kubectl", "get", "-o", "json", "-n", namespace, resourceRef)
	errB := bytes.NewBuffer([]byte{})
	outB := bytes.NewBuffer([]byte{})
	c.Stderr = errB
	c.Stdout = outB
	if err := c.Run(); err != nil {
		return nil, errors.NewEf(err, errB.String())
	}
	return outB.Bytes(), nil
}

func KubectlDelete(namespace, resourceRef string) error {
	c := exec.Command("kubectl", "delete", "-n", namespace, resourceRef)
	errB := bytes.NewBuffer([]byte{})
	outB := bytes.NewBuffer([]byte{})
	c.Stderr = errB
	c.Stdout = outB
	if err := c.Run(); err != nil {
		return errors.NewEf(err, errB.String())
	}
	return nil
}

func AsOwner(r client.Object, controller ...bool) metav1.OwnerReference {
	ctrller := false
	if len(controller) > 0 {
		ctrller = true
	}
	return metav1.OwnerReference{
		APIVersion:         r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Kind:               r.GetObjectKind().GroupVersionKind().Kind,
		Name:               r.GetName(),
		UID:                r.GetUID(),
		Controller:         &ctrller,
		BlockOwnerDeletion: NewBool(true),
	}
}

func IsOwner(obj client.Object, ownerRef metav1.OwnerReference) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Name == ownerRef.Name &&
			ref.UID == ownerRef.UID &&
			ref.Kind == ownerRef.Kind && ref.
			APIVersion == ownerRef.APIVersion {
			return true
		}
	}
	return false
}

func KubectlWithConfig(command string, config []byte) ([]byte, error) {
	filename, err := uuid.GenerateUUID()
	if err != nil {
		return nil, err
	}

	s := os.TempDir()
	fname := fmt.Sprintf("%s/%s.conf", s, filename)
	if err := os.WriteFile(fname, config, os.ModePerm); err != nil {
		return nil, err
	}
	defer os.Remove(fname)

	r := csv.NewReader(strings.NewReader(command))
	r.Comma = ' '
	cmdArr, err := r.Read()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("kubectl", cmdArr...)
	cmd.Stderr = os.Stderr
	cmd.Env = append(cmd.Env, fmt.Sprintf("KUBECONFIG=%s", fname))

	return cmd.Output()
}

func NamespacedName(obj client.Object) types.NamespacedName {
	return types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
}

func KubectlApplyExecWithConfig(stdin []byte, config []byte) (stdout *bytes.Buffer, err error) {

	filename, err := uuid.GenerateUUID()
	if err != nil {
		return nil, err
	}

	s := os.TempDir()
	fname := fmt.Sprintf("%s/%s.conf", s, filename)
	if err := os.WriteFile(fname, config, os.ModePerm); err != nil {
		return nil, err
	}
	defer os.Remove(fname)

	c := exec.Command("kubectl", "apply", "-f", "-")
	outStream, errStream := bytes.NewBuffer([]byte{}), bytes.NewBuffer([]byte{})
	c.Stdin = bytes.NewBuffer(stdin)
	c.Stdout = outStream
	c.Stderr = errStream
	c.Env = append(c.Env, fmt.Sprintf("KUBECONFIG=%s", fname))
	if err := c.Run(); err != nil {
		return outStream, errors.NewEf(err, errStream.String())
	}
	fmt.Printf("stdout: %s\n", outStream.Bytes())
	return outStream, nil
}
