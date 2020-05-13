package machine

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	mapierrors "github.com/openshift/machine-api-operator/pkg/controller/machine"
	"k8s.io/klog"
	awsproviderv1 "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1beta1"
	awsclient "sigs.k8s.io/cluster-api-provider-aws/pkg/client"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	kubevirtclient "sigs.k8s.io/cluster-api-provider-kubevirt/pkg/client"
)

// Scan machine tags, and return a deduped tags list
func removeDuplicatedTags(tags []*ec2.Tag) []*ec2.Tag {
	m := make(map[string]bool)
	result := []*ec2.Tag{}

	// look for duplicates
	for _, entry := range tags {
		if _, value := m[*entry.Key]; !value {
			m[*entry.Key] = true
			result = append(result, entry)
		}
	}
	return result
}

// removeStoppedMachine removes all instances of a specific machine that are in a stopped state.
func removeStoppedMachine(machine *machinev1.Machine, client awsclient.Client) error {
	instances, err := getStoppedInstances(machine, client)
	if err != nil {
		klog.Errorf("Error getting stopped instances: %v", err)
		return fmt.Errorf("error getting stopped instances: %v", err)
	}

	if len(instances) == 0 {
		klog.Infof("No stopped instances found for machine %v", machine.Name)
		return nil
	}

	_, err = terminateInstances(client, instances)
	return err
}

func buildEC2Filters(inputFilters []awsproviderv1.Filter) []*ec2.Filter {
	filters := make([]*ec2.Filter, len(inputFilters))
	for i, f := range inputFilters {
		values := make([]*string, len(f.Values))
		for j, v := range f.Values {
			values[j] = aws.String(v)
		}
		filters[i] = &ec2.Filter{
			Name:   aws.String(f.Name),
			Values: values,
		}
	}
	return filters
}

func getSecurityGroupsIDs(securityGroups []awsproviderv1.AWSResourceReference, client awsclient.Client) ([]*string, error) {
	var securityGroupIDs []*string
	for _, g := range securityGroups {
		// ID has priority
		if g.ID != nil {
			securityGroupIDs = append(securityGroupIDs, g.ID)
		} else if g.Filters != nil {
			klog.Info("Describing security groups based on filters")
			// Get groups based on filters
			describeSecurityGroupsRequest := ec2.DescribeSecurityGroupsInput{
				Filters: buildEC2Filters(g.Filters),
			}
			describeSecurityGroupsResult, err := client.DescribeSecurityGroups(&describeSecurityGroupsRequest)
			if err != nil {
				klog.Errorf("error describing security groups: %v", err)
				return nil, fmt.Errorf("error describing security groups: %v", err)
			}
			for _, g := range describeSecurityGroupsResult.SecurityGroups {
				groupID := *g.GroupId
				securityGroupIDs = append(securityGroupIDs, &groupID)
			}
		}
	}

	if len(securityGroups) == 0 {
		klog.Info("No security group found")
	}

	return securityGroupIDs, nil
}

func getSubnetIDs(subnet awsproviderv1.AWSResourceReference, availabilityZone string, client awsclient.Client) ([]*string, error) {
	var subnetIDs []*string
	// ID has priority
	if subnet.ID != nil {
		subnetIDs = append(subnetIDs, subnet.ID)
	} else {
		var filters []awsproviderv1.Filter
		if availabilityZone != "" {
			// Improve error logging for better user experience.
			// Otherwise, during the process of minimizing API calls, this is a good
			// candidate for removal.
			_, err := client.DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{
				ZoneNames: []*string{aws.String(availabilityZone)},
			})
			if err != nil {
				klog.Errorf("error describing availability zones: %v", err)
				return nil, fmt.Errorf("error describing availability zones: %v", err)
			}
			filters = append(filters, awsproviderv1.Filter{Name: "availabilityZone", Values: []string{availabilityZone}})
		}
		filters = append(filters, subnet.Filters...)
		klog.Info("Describing subnets based on filters")
		describeSubnetRequest := ec2.DescribeSubnetsInput{
			Filters: buildEC2Filters(filters),
		}
		describeSubnetResult, err := client.DescribeSubnets(&describeSubnetRequest)
		if err != nil {
			klog.Errorf("error describing subnetes: %v", err)
			return nil, fmt.Errorf("error describing subnets: %v", err)
		}
		for _, n := range describeSubnetResult.Subnets {
			subnetID := *n.SubnetId
			subnetIDs = append(subnetIDs, &subnetID)
		}
	}
	if len(subnetIDs) == 0 {
		return nil, fmt.Errorf("no subnet IDs were found")
	}
	return subnetIDs, nil
}

func getAMI(AMI awsproviderv1.AWSResourceReference, client awsclient.Client) (*string, error) {
	kubevirtclient.Client()
	if AMI.ID != nil {
		amiID := AMI.ID
		klog.Infof("Using AMI %s", *amiID)
		return amiID, nil
	}
	if len(AMI.Filters) > 0 {
		klog.Info("Describing AMI based on filters")
		describeImagesRequest := ec2.DescribeImagesInput{
			Filters: buildEC2Filters(AMI.Filters),
		}
		describeAMIResult, err := client.DescribeImages(&describeImagesRequest)
		if err != nil {
			klog.Errorf("error describing AMI: %v", err)
			return nil, fmt.Errorf("error describing AMI: %v", err)
		}
		if len(describeAMIResult.Images) < 1 {
			klog.Errorf("no image for given filters not found")
			return nil, fmt.Errorf("no image for given filters not found")
		}
		latestImage := describeAMIResult.Images[0]
		latestTime, err := time.Parse(time.RFC3339, *latestImage.CreationDate)
		if err != nil {
			klog.Errorf("unable to parse time for %q AMI: %v", *latestImage.ImageId, err)
			return nil, fmt.Errorf("unable to parse time for %q AMI: %v", *latestImage.ImageId, err)
		}
		for _, image := range describeAMIResult.Images[1:] {
			imageTime, err := time.Parse(time.RFC3339, *image.CreationDate)
			if err != nil {
				klog.Errorf("unable to parse time for %q AMI: %v", *image.ImageId, err)
				return nil, fmt.Errorf("unable to parse time for %q AMI: %v", *image.ImageId, err)
			}
			if latestTime.Before(imageTime) {
				latestImage = image
				latestTime = imageTime
			}
		}
		return latestImage.ImageId, nil
	}
	return nil, fmt.Errorf("AMI ID or AMI filters need to be specified")
}

func getBlockDeviceMappings(blockDeviceMappings []awsproviderv1.BlockDeviceMappingSpec, AMI string, client awsclient.Client) ([]*ec2.BlockDeviceMapping, error) {
	if len(blockDeviceMappings) == 0 || blockDeviceMappings[0].EBS == nil {
		return []*ec2.BlockDeviceMapping{}, nil
	}

	// Get image object to get the RootDeviceName
	describeImagesRequest := ec2.DescribeImagesInput{
		ImageIds: []*string{&AMI},
	}
	describeAMIResult, err := client.DescribeImages(&describeImagesRequest)
	if err != nil {
		klog.Errorf("Error describing AMI: %v", err)
		return nil, fmt.Errorf("error describing AMI: %v", err)
	}
	if len(describeAMIResult.Images) < 1 {
		klog.Errorf("No image for given AMI was found")
		return nil, fmt.Errorf("no image for given AMI not found")
	}
	deviceName := describeAMIResult.Images[0].RootDeviceName

	// Only support one blockDeviceMapping
	volumeSize := blockDeviceMappings[0].EBS.VolumeSize
	volumeType := blockDeviceMappings[0].EBS.VolumeType
	blockDeviceMapping := ec2.BlockDeviceMapping{
		DeviceName: deviceName,
		Ebs: &ec2.EbsBlockDevice{
			VolumeSize: volumeSize,
			VolumeType: volumeType,
			Encrypted:  blockDeviceMappings[0].EBS.Encrypted,
		},
	}

	if aws.StringValue(blockDeviceMappings[0].EBS.KMSKey.ID) != "" {
		klog.V(3).Infof("Using KMS key ID %q for encrypting EBS volume", *blockDeviceMappings[0].EBS.KMSKey.ID)
		blockDeviceMapping.Ebs.KmsKeyId = blockDeviceMappings[0].EBS.KMSKey.ID
	} else if aws.StringValue(blockDeviceMappings[0].EBS.KMSKey.ARN) != "" {
		klog.V(3).Info("Using KMS key ARN for encrypting EBS volume") // ARN usually have account ids, therefore are sensitive data so shouldn't log the value
		blockDeviceMapping.Ebs.KmsKeyId = blockDeviceMappings[0].EBS.KMSKey.ARN
	}

	if *volumeType == "io1" {
		blockDeviceMapping.Ebs.Iops = blockDeviceMappings[0].EBS.Iops
	}

	return []*ec2.BlockDeviceMapping{&blockDeviceMapping}, nil
}

func createVm(machine *machinev1.Machine, machineProviderConfig *awsproviderv1.AWSMachineProviderConfig, userData []byte, client awsclient.Client) (*ec2.Instance, error) {
	amiID, err := getAMI(machineProviderConfig.AMI, client)
	if err != nil {
		return nil, mapierrors.InvalidMachineConfiguration("error getting AMI: %v", err)
	}

	securityGroupsIDs, err := getSecurityGroupsIDs(machineProviderConfig.SecurityGroups, client)
	if err != nil {
		return nil, mapierrors.InvalidMachineConfiguration("error getting security groups IDs: %v", err)
	}
	subnetIDs, err := getSubnetIDs(machineProviderConfig.Subnet, machineProviderConfig.Placement.AvailabilityZone, client)
	if err != nil {
		return nil, mapierrors.InvalidMachineConfiguration("error getting subnet IDs: %v", err)
	}
	if len(subnetIDs) > 1 {
		klog.Warningf("More than one subnet id returned, only first one will be used")
	}

	// build list of networkInterfaces (just 1 for now)
	var networkInterfaces = []*ec2.InstanceNetworkInterfaceSpecification{
		{
			DeviceIndex:              aws.Int64(machineProviderConfig.DeviceIndex),
			AssociatePublicIpAddress: machineProviderConfig.PublicIP,
			SubnetId:                 subnetIDs[0],
			Groups:                   securityGroupsIDs,
		},
	}

	blockDeviceMappings, err := getBlockDeviceMappings(machineProviderConfig.BlockDevices, *amiID, client)
	if err != nil {
		return nil, mapierrors.InvalidMachineConfiguration("error getting blockDeviceMappings: %v", err)
	}

	clusterID, ok := getClusterID(machine)
	if !ok {
		klog.Errorf("Unable to get cluster ID for machine: %q", machine.Name)
		return nil, mapierrors.InvalidMachineConfiguration("Unable to get cluster ID for machine: %q", machine.Name)
	}
	// Add tags to the created machine
	rawTagList := []*ec2.Tag{}
	for _, tag := range machineProviderConfig.Tags {
		rawTagList = append(rawTagList, &ec2.Tag{Key: aws.String(tag.Name), Value: aws.String(tag.Value)})
	}
	rawTagList = append(rawTagList, []*ec2.Tag{
		{Key: aws.String("kubernetes.io/cluster/" + clusterID), Value: aws.String("owned")},
		{Key: aws.String("Name"), Value: aws.String(machine.Name)},
	}...)
	tagList := removeDuplicatedTags(rawTagList)
	tagInstance := &ec2.TagSpecification{
		ResourceType: aws.String("instance"),
		Tags:         tagList,
	}
	tagVolume := &ec2.TagSpecification{
		ResourceType: aws.String("volume"),
		Tags:         tagList,
	}

	userDataEnc := base64.StdEncoding.EncodeToString(userData)

	var iamInstanceProfile *ec2.IamInstanceProfileSpecification
	if machineProviderConfig.IAMInstanceProfile != nil && machineProviderConfig.IAMInstanceProfile.ID != nil {
		iamInstanceProfile = &ec2.IamInstanceProfileSpecification{
			Name: aws.String(*machineProviderConfig.IAMInstanceProfile.ID),
		}
	}

	var placement *ec2.Placement
	if machineProviderConfig.Placement.AvailabilityZone != "" && machineProviderConfig.Subnet.ID == nil {
		placement = &ec2.Placement{
			AvailabilityZone: aws.String(machineProviderConfig.Placement.AvailabilityZone),
		}
	}

	inputConfig := ec2.RunInstancesInput{
		ImageId:      amiID,
		InstanceType: aws.String(machineProviderConfig.InstanceType),
		// Only a single instance of the AWS instance allowed
		MinCount:              aws.Int64(1),
		MaxCount:              aws.Int64(1),
		KeyName:               machineProviderConfig.KeyName,
		IamInstanceProfile:    iamInstanceProfile,
		TagSpecifications:     []*ec2.TagSpecification{tagInstance, tagVolume},
		NetworkInterfaces:     networkInterfaces,
		UserData:              &userDataEnc,
		Placement:             placement,
		InstanceMarketOptions: getInstanceMarketOptionsRequest(machineProviderConfig),
	}

	if len(blockDeviceMappings) > 0 {
		inputConfig.BlockDeviceMappings = blockDeviceMappings
	}
	runResult, err := client.RunInstances(&inputConfig)
	if err != nil {
		// we return InvalidMachineConfiguration for 4xx errors which by convention signal client misconfiguration
		// https://tools.ietf.org/html/rfc2616#section-6.1.1
		// https: //docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html
		// https://docs.aws.amazon.com/sdk-for-go/api/aws/awserr/
		if _, ok := err.(awserr.Error); ok {
			if reqErr, ok := err.(awserr.RequestFailure); ok {
				if strings.HasPrefix(strconv.Itoa(reqErr.StatusCode()), "4") {
					klog.Infof("Error launching instance: %v", reqErr)
					return nil, mapierrors.InvalidMachineConfiguration("error launching instance: %v", reqErr.Message())
				}
			}
		}
		klog.Errorf("Error creating EC2 instance: %v", err)
		return nil, mapierrors.CreateMachine("error creating EC2 instance: %v", err)
	}

	if runResult == nil || len(runResult.Instances) != 1 {
		klog.Errorf("Unexpected reservation creating instances: %v", runResult)
		return nil, mapierrors.CreateMachine("unexpected reservation creating instance")
	}

	return runResult.Instances[0], nil
}

type instanceList []*ec2.Instance

func (il instanceList) Len() int {
	return len(il)
}

func (il instanceList) Swap(i, j int) {
	il[i], il[j] = il[j], il[i]
}

func (il instanceList) Less(i, j int) bool {
	if il[i].LaunchTime == nil && il[j].LaunchTime == nil {
		return false
	}
	if il[i].LaunchTime != nil && il[j].LaunchTime == nil {
		return false
	}
	if il[i].LaunchTime == nil && il[j].LaunchTime != nil {
		return true
	}
	return (*il[i].LaunchTime).After(*il[j].LaunchTime)
}

// sortInstances will sort a list of instance based on an instace launch time
// from the newest to the oldest.
// This function should only be called with running instances, not those which are stopped or
// terminated.
func sortInstances(instances []*ec2.Instance) {
	sort.Sort(instanceList(instances))
}

func getInstanceMarketOptionsRequest(providerConfig *awsproviderv1.AWSMachineProviderConfig) *ec2.InstanceMarketOptionsRequest {
	if providerConfig.SpotMarketOptions == nil {
		// Instance is not a Spot instance
		return nil
	}

	// Set required values for Spot instances
	spotOptions := &ec2.SpotMarketOptions{}
	// The following two options ensure that:
	// - If an instance is interrupted, it is terminated rather than hibernating or stopping
	// - No replacement instance will be created if the instance is interrupted
	// - If the spot request cannot immediately be fulfilled, it will not be created
	// This behaviour should satisfy the 1:1 mapping of Machines to Instances as
	// assumed by the machine API.
	spotOptions.SetInstanceInterruptionBehavior(ec2.InstanceInterruptionBehaviorTerminate)
	spotOptions.SetSpotInstanceType(ec2.SpotInstanceTypeOneTime)

	// Set the MaxPrice if specified by the providerConfig
	maxPrice := providerConfig.SpotMarketOptions.MaxPrice
	if maxPrice != nil && *maxPrice != "" {
		spotOptions.SetMaxPrice(*maxPrice)
	}

	instanceMarketOptionsRequest := &ec2.InstanceMarketOptionsRequest{}
	instanceMarketOptionsRequest.SetMarketType(ec2.MarketTypeSpot)
	instanceMarketOptionsRequest.SetSpotOptions(spotOptions)

	return instanceMarketOptionsRequest
}
