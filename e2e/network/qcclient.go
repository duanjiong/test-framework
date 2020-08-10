package network

import (
	"fmt"
	qcconfig "github.com/yunify/qingcloud-sdk-go/config"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog"
)

const (
	QYConfigPath = "/etc/qingcloud/config.yaml"
)

type QingCloud struct {
	instanceService      *qcservice.InstanceService
	lbService            *qcservice.LoadBalancerService
	volumeService        *qcservice.VolumeService
	jobService           *qcservice.JobService
	eipService           *qcservice.EIPService
	securityGroupService *qcservice.SecurityGroupService
	tagService           *qcservice.TagService
}

// newQingCloud returns a new instance of QingCloud cloud provider.
func newQingCloud() (*QingCloud, error) {
	qcConfig, err := qcconfig.NewDefault()
	if err != nil {
		return nil, err
	}
	if err = qcConfig.LoadConfigFromFilepath(QYConfigPath); err != nil {
		return nil, err
	}
	qcService, err := qcservice.Init(qcConfig)
	if err != nil {
		return nil, err
	}
	instanceService, err := qcService.Instance(qcConfig.Zone)
	if err != nil {
		return nil, err
	}
	eipService, _ := qcService.EIP(qcConfig.Zone)
	lbService, err := qcService.LoadBalancer(qcConfig.Zone)
	if err != nil {
		return nil, err
	}
	volumeService, err := qcService.Volume(qcConfig.Zone)
	if err != nil {
		return nil, err
	}
	jobService, err := qcService.Job(qcConfig.Zone)
	if err != nil {
		return nil, err
	}
	securityGroupService, err := qcService.SecurityGroup(qcConfig.Zone)
	if err != nil {
		return nil, err
	}
	api, _ := qcService.Accesskey(qcConfig.Zone)
	output, err := api.DescribeAccessKeys(&qcservice.DescribeAccessKeysInput{
		AccessKeys: []*string{&qcConfig.AccessKeyID},
	})
	if err != nil {
		klog.Errorf("Failed to get userID")
		return nil, err
	}
	if len(output.AccessKeySet) == 0 {
		err = fmt.Errorf("AccessKey %s have not userid", qcConfig.AccessKeyID)
		return nil, err
	}
	qc := QingCloud{
		instanceService:      instanceService,
		lbService:            lbService,
		volumeService:        volumeService,
		jobService:           jobService,
		securityGroupService: securityGroupService,
		eipService:           eipService,
	}

	return &qc, nil
}
