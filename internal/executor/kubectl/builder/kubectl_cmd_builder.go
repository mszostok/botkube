package builder

import (
	"context"
	"errors"
	"fmt"
	"github.com/kubeshop/botkube/pkg/api"
	"github.com/kubeshop/botkube/pkg/execute"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubeshop/botkube/pkg/config"
	"github.com/kubeshop/botkube/pkg/execute/kubectl"
)

var errUnsupportedCommand = errors.New("unsupported command")

const (
	verbsDropdownCommand             = "kubectl @builder --verbs"
	resourceTypesDropdownCommand     = "kubectl @builder --resource-type"
	resourceNamesDropdownCommand     = "kubectl @builder --resource-name"
	resourceNamespaceDropdownCommand = "kubectl @builder --namespace"
	filterPlaintextInputCommand      = "kubectl @builder --filter-query"
	kubectlCommandName               = "kubectl"
	dropdownItemsLimit               = 100
	noKubectlCommandsInChannel       = "No `kubectl` commands are enabled in this channel. To learn how to enable them, visit https://docs.botkube.io/configuration/executor."
	kubectlMissingCommandMsg         = "Please specify the kubectl command"
)

var knownCmdPrefix = map[string]struct{}{
	verbsDropdownCommand:             {},
	resourceTypesDropdownCommand:     {},
	resourceNamesDropdownCommand:     {},
	resourceNamespaceDropdownCommand: {},
	filterPlaintextInputCommand:      {},
}

var errRequiredVerbDropdown = errors.New("verbs dropdown select cannot be empty")

type (
	kcMerger interface {
		MergeAllEnabled() kubectl.EnabledKubectl
	}

	// NamespaceLister provides an option to list all namespaces in a given cluster.
	NamespaceLister interface {
		List(ctx context.Context, opts metav1.ListOptions) (*corev1.NamespaceList, error)
	}

	kcRunner interface {
		RunKubectlCommand(ctx context.Context, defaultNamespace, cmd string) (string, error)
	}
)

// KubectlCmdBuilder provides functionality to handle interactive kubectl command selection.
type KubectlCmdBuilder struct {
	log             logrus.FieldLogger
	NamespaceLister NamespaceLister
	CommandGuard    execute.CommandGuard
	kcRunner        kcRunner
}

// NewKubectlCmdBuilder returns a new KubectlCmdBuilder instance.
func NewKubectlCmdBuilder(log logrus.FieldLogger, executor kcRunner) *KubectlCmdBuilder {
	return &KubectlCmdBuilder{
		log:      log,
		kcRunner: executor,
	}
}

// Do executes a given kc-cmd-builder command based on args.
//
// TODO: once we will have a real use-case, we should abstract the Slack state and introduce our own model.
func (e *KubectlCmdBuilder) Do(ctx context.Context, l *logrus.Logger, cmd, defaultNamespace string, platform config.CommPlatformIntegration, state *slack.BlockActionStates, botName string) (api.Message, error) {
	var empty api.Message

	e.log = l
	if platform != config.SocketSlackCommPlatformIntegration {
		e.log.Debug("Interactive kubectl command builder is not supported on %s platform", platform)
		return e.message(kubectlMissingCommandMsg)
	}

	allVerbs, allTypes := e.getEnableKubectlDetails()
	if len(allVerbs) == 0 {
		return e.message(noKubectlCommandsInChannel)
	}

	allVerbs = e.CommandGuard.FilterSupportedVerbs(allVerbs)

	if cmd == "" { // return initial command builder message
		return e.initialMessage(botName, allVerbs)
	}

	stateDetails := e.extractStateDetails(botName, state)
	if stateDetails.namespace == "" {
		stateDetails.namespace = defaultNamespace
	}

	e.log.Info("state", stateDetails.namespace)
	e.log.Info("state", stateDetails.resourceName)
	e.log.Info("state", stateDetails.resourceType)
	e.log.Info("state", stateDetails.verb)
	cmds := executorsRunner{
		verbsDropdownCommand: func() (api.Message, error) {
			return e.renderMessage(ctx, botName, stateDetails, allVerbs, allTypes)
		},
		resourceTypesDropdownCommand: func() (api.Message, error) {
			// the resource type was selected, so clear resource name from command preview.
			stateDetails.resourceName = ""
			e.log.Info("Selecting resource type")
			return e.renderMessage(ctx, botName, stateDetails, allVerbs, allTypes)
		},
		resourceNamesDropdownCommand: func() (api.Message, error) {
			// this is called only when the resource name is directly selected from dropdown, so we need to include
			// it in command preview.
			return e.renderMessage(ctx, botName, stateDetails, allVerbs, allTypes)
		},
		resourceNamespaceDropdownCommand: func() (api.Message, error) {
			// when the namespace was changed, there is a small chance that resource name will be still matching,
			// we will need to do the external call to check that. For now, we clear resource name from command preview.
			stateDetails.resourceName = ""
			return e.renderMessage(ctx, botName, stateDetails, allVerbs, allTypes)
		},
		filterPlaintextInputCommand: func() (api.Message, error) {
			return e.renderMessage(ctx, botName, stateDetails, allVerbs, allTypes)
		},
	}

	args := strings.Fields(cmd)
	cmd = fmt.Sprintf("kubectl %s %s", args[0], args[1])
	msg, err := cmds.SelectAndRun(cmd)
	if err != nil {
		e.log.WithField("error", err.Error()).Error("Cannot render the kubectl command builder. Returning empty message.")
		return empty, err
	}
	return msg, nil
}

func (e *KubectlCmdBuilder) initialMessage(botName string, allVerbs []string) (api.Message, error) {
	var empty api.Message

	// We start a new interactive block, so we generate unique ID.
	// Later when we update this message with a new "body" e.g. update command preview
	// the block state remains the same as Slack always see it under the same id.
	// If we use different ID each time we update the message, Slack will clean up the state
	// meaning we will lose information about verb/resourceType/resourceName that were previously selected.
	id, err := uuid.NewRandom()
	if err != nil {
		return empty, err
	}
	allVerbsSelect := VerbSelect(botName, allVerbs, "")
	if allVerbsSelect == nil {
		return empty, errRequiredVerbDropdown
	}

	msg := KubectlCmdBuilderMessage(id.String(), *allVerbsSelect)
	// we are the initial message, don't replace the original one as we need to send a brand-new message visible only to the user
	// otherwise we can replace a message that is publicly visible.
	msg.ReplaceOriginal = false

	return msg, nil
}

func (e *KubectlCmdBuilder) renderMessage(ctx context.Context, botName string, stateDetails stateDetails, allVerbs, allTypes []string) (api.Message, error) {
	var empty api.Message

	allVerbsSelect := VerbSelect(botName, allVerbs, stateDetails.verb)
	if allVerbsSelect == nil {
		return empty, errRequiredVerbDropdown
	}

	// 1. Refresh resource type list
	matchingTypes, err := e.getAllowedResourcesSelectList(botName, stateDetails.verb, allTypes, stateDetails.resourceType)
	if err != nil {
		return empty, err
	}

	// 2. If a given verb doesn't have assigned resource types,
	//    render:
	//      1. Dropdown with all verbs
	//      2. Filter input
	//      3. Command preview. For example:
	//           kubectl api-resources
	if matchingTypes == nil {
		// we must zero those fields as they are known only if we know the resource type and this verb doesn't have one :)
		stateDetails.resourceType = ""
		stateDetails.resourceName = ""
		stateDetails.namespace = ""
		preview := e.buildCommandPreview(botName, stateDetails)

		return KubectlCmdBuilderMessage(
			stateDetails.dropdownsBlockID, *allVerbsSelect,
			WithAdditionalSections(preview...),
		), nil
	}

	// 3. If resource type is not on the list anymore,
	//    render:
	//      1. Dropdown with all verbs
	//      2. Dropdown with all related resource types
	//    because we don't know the resource type we cannot render:
	//      1. Resource names - obvious :).
	//      2. Namespaces as we don't know if it's cluster or namespace scoped resource.
	if !e.contains(matchingTypes, stateDetails.resourceType) {
		return KubectlCmdBuilderMessage(
			stateDetails.dropdownsBlockID, *allVerbsSelect,
			WithAdditionalSelects(matchingTypes),
		), nil
	}

	// At this stage we know that:
	//   1. Verb requires resource types
	//   2. Selected resource type is still valid for the selected verb
	var (
		resNames = e.tryToGetResourceNamesSelect(botName, stateDetails)
		nsNames  = e.tryToGetNamespaceSelect(ctx, botName, stateDetails)
	)

	// 4. If a given resource name is not on the list anymore, clear it.
	if !e.contains(resNames, stateDetails.resourceName) {
		stateDetails.resourceName = ""
	}

	// 5. If a given namespace is not on the list anymore, clear it.
	if !e.contains(nsNames, stateDetails.namespace) {
		stateDetails.namespace = ""
	}

	// 6. Render all dropdowns and full command preview.
	preview := e.buildCommandPreview(botName, stateDetails)
	return KubectlCmdBuilderMessage(
		stateDetails.dropdownsBlockID, *allVerbsSelect,
		WithAdditionalSelects(matchingTypes, resNames, nsNames),
		WithAdditionalSections(preview...),
	), nil
}

func (e *KubectlCmdBuilder) tryToGetResourceNamesSelect(botName string, state stateDetails) *api.Select {
	e.log.Info("Get resource names")
	if state.resourceType == "" {
		e.log.Info("Return empty resource name")
		return EmptyResourceNameDropdown()
	}
	cmd := fmt.Sprintf(`get %s --ignore-not-found=true -o go-template='{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}'`, state.resourceType)
	if state.namespace != "" {
		cmd = fmt.Sprintf("%s -n %s", cmd, state.namespace)
	}
	e.log.Infof("Run cmd %q", cmd)

	out, err := e.kcRunner.RunKubectlCommand(context.Background(), "default", cmd) // TODO: context
	if err != nil {
		e.log.WithField("error", err.Error()).Error("Cannot fetch resource names. Returning empty resource name dropdown.")
		return EmptyResourceNameDropdown()
	}
	e.log.Infof("Got out %q", out)

	lines := getNonEmptyLines(out)
	if len(lines) == 0 {
		return EmptyResourceNameDropdown()
	}

	return ResourceNamesSelect(botName, overflowSentence(lines), state.resourceName)
}

func (e *KubectlCmdBuilder) tryToGetNamespaceSelect(ctx context.Context, botName string, details stateDetails) *api.Select {
	log := e.log.WithFields(logrus.Fields{
		"state": details,
	})

	resourceDetails, err := e.CommandGuard.GetResourceDetails(details.verb, details.resourceType)
	if err != nil {
		log.WithField("error", err.Error()).Error("Cannot fetch resource details, ignoring namespace dropdown...")
		return nil
	}

	if !resourceDetails.Namespaced {
		log.Debug("Resource is not namespace-scoped, ignore namespace dropdown...")
		return nil
	}

	allClusterNamespaces, err := e.NamespaceLister.List(ctx, metav1.ListOptions{
		Limit: dropdownItemsLimit,
	})
	if err != nil {
		log.WithField("error", err.Error()).Error("Cannot fetch all available Kubernetes namespaces, ignoring namespace dropdown...")
		return nil
	}

	var (
		//kc        = e.merger.MergeAllEnabled()
		//allowedNS = kc.AllowedNamespacesPerResource[details.resourceType]
		finalNS []dropdownItem
	)

	initialNamespace := newDropdownItem(details.namespace, details.namespace)
	initialNamespace = e.appendNamespaceSuffixIfDefault(initialNamespace)

	for _, item := range allClusterNamespaces.Items {
		//if !allowedNS.IsAllowed(item.Name) {
		//	log.WithField("namespace", item.Name).Debug("Namespace is not allowed, so skipping it.")
		//	continue
		//}

		kv := newDropdownItem(item.Name, item.Name)
		if initialNamespace.Value == kv.Value {
			kv = e.appendNamespaceSuffixIfDefault(kv)
		}

		finalNS = append(finalNS, kv)
	}

	return ResourceNamespaceSelect(botName, finalNS, initialNamespace)
}

// UX requirement to append the (namespace) suffix if the namespace is called `default`.
func (e *KubectlCmdBuilder) appendNamespaceSuffixIfDefault(in dropdownItem) dropdownItem {
	if in.Name == "default" {
		in.Name += " (namespace)"
	}
	return in
}

func (e *KubectlCmdBuilder) getEnableKubectlDetails() ([]string, []string) {
	// TODO: take from can-i or from configuration...
	verbs := []string{
		"api-resources", "api-versions", "cluster-info", "describe", "explain", "get", "logs", "top",
	}
	resources := []string{
		"deployments", "pods", "namespaces", "daemonsets", "statefulsets", "storageclasses", "nodes", "configmaps", "services", "ingresses",
	}
	return verbs, resources
	//enabledKubectls := e.merger.MergeAllEnabled(bindings)
	//for key := range enabledKubectls.AllowedKubectlResource {
	//	resources = append(resources, key)
	//}
	//sort.Strings(resources)
	//
	//for key := range enabledKubectls.AllowedKubectlVerb {
	//	verbs = append(verbs, key)
	//}
	//sort.Strings(verbs)
	//
	//if enabledKubectls.DefaultNamespace == "" {
	//	enabledKubectls.DefaultNamespace = kubectlDefaultNamespace
	//}
	//
	//return verbs, resources, enabledKubectls.DefaultNamespace
}

// getAllowedResourcesSelectList returns dropdown select with allowed resources for a given verb.
func (e *KubectlCmdBuilder) getAllowedResourcesSelectList(botName, verb string, resources []string, resourceType string) (*api.Select, error) {
	allowedResources, err := e.CommandGuard.GetAllowedResourcesForVerb(verb, resources)
	if err != nil {
		return nil, err
	}
	if len(allowedResources) == 0 {
		return nil, nil
	}

	allowedResourcesList := make([]string, 0, len(allowedResources))
	for _, item := range allowedResources {
		allowedResourcesList = append(allowedResourcesList, item.Name)
	}

	return ResourceTypeSelect(botName, allowedResourcesList, resourceType), nil
}

type stateDetails struct {
	dropdownsBlockID string

	verb         string
	namespace    string
	resourceType string
	resourceName string
	filter       string
}

func (e *KubectlCmdBuilder) extractStateDetails(botName string, state *slack.BlockActionStates) stateDetails {
	if state == nil {
		return stateDetails{}
	}

	details := stateDetails{}
	for blockID, blocks := range state.Values {
		if !strings.Contains(blockID, filterPlaintextInputCommand) {
			details.dropdownsBlockID = blockID
		}
		for id, act := range blocks {
			//id = strings.TrimPrefix(id, botName)
			_, id, _ = strings.Cut(id, " ")
			id = strings.TrimSpace(id)

			switch id {
			case verbsDropdownCommand:
				details.verb = act.SelectedOption.Value
			case resourceTypesDropdownCommand:
				details.resourceType = act.SelectedOption.Value
			case resourceNamesDropdownCommand:
				details.resourceName = act.SelectedOption.Value
			case resourceNamespaceDropdownCommand:
				details.namespace = act.SelectedOption.Value
			case filterPlaintextInputCommand:
				details.filter = act.Value
			}
		}
	}
	return details
}

func (e *KubectlCmdBuilder) contains(matchingTypes *api.Select, resourceType string) bool {
	if matchingTypes == nil {
		return false
	}

	if matchingTypes.InitialOption != nil && matchingTypes.InitialOption.Value == resourceType {
		return true
	}

	return false
}

func (e *KubectlCmdBuilder) buildCommandPreview(botName string, state stateDetails) []api.Section {
	resourceDetails, err := e.CommandGuard.GetResourceDetails(state.verb, state.resourceType)
	if err != nil {
		e.log.WithFields(logrus.Fields{
			"state": state,
			"error": err.Error(),
		}).Error("Cannot get resource details")
		return []api.Section{InternalErrorSection()}
	}

	if resourceDetails.SlashSeparatedInCommand && state.resourceName == "" {
		// we should not render the command as it will be invalid anyway without the resource name
		return nil
	}

	cmd := fmt.Sprintf("%s %s %s", kubectlCommandName, state.verb, state.resourceType)

	resourceNameSeparator := " "
	if resourceDetails.SlashSeparatedInCommand {
		// sometimes kubectl commands requires slash separator, without it, it will not work. For example:
		//   kubectl logs deploy/<deploy_name>
		resourceNameSeparator = "/"
	}

	if state.resourceName != "" {
		cmd = fmt.Sprintf("%s%s%s", cmd, resourceNameSeparator, state.resourceName)
	}

	if resourceDetails.Namespaced && state.namespace != "" {
		cmd = fmt.Sprintf("%s -n %s", cmd, state.namespace)
	}

	if state.filter != "" {
		cmd = fmt.Sprintf("%s --filter=%q", cmd, state.filter)
	}

	return PreviewSection(botName, cmd, FilterSection(botName))
}

func (e *KubectlCmdBuilder) message(msg string) (api.Message, error) {
	return api.Message{
		Base: api.Base{
			Body: api.Body{
				Plaintext: msg,
			},
		},
	}, nil
}

func splitByNewLines(c rune) bool {
	return c == '\n' || c == '\r'
}

func overflowSentence(in []string) []string {
	for idx := range in {
		if len(in[idx]) < 76 { // Maximum length for text field in dropdown is 75 characters. (https://api.slack.com/reference/block-kit/composition-objects#option)
			continue
		}

		in[idx] = in[idx][:72] + "..."
	}
	return in
}

func getNonEmptyLines(in string) []string {
	lines := strings.FieldsFunc(in, splitByNewLines)
	var out []string
	for _, x := range lines {
		if x == "" {
			continue
		}
		out = append(out, x)
	}
	return out
}

type (
	executorFunc    func() (api.Message, error)
	executorsRunner map[string]executorFunc
)

func (cmds executorsRunner) SelectAndRun(cmdVerb string) (api.Message, error) {
	cmdVerb = strings.ToLower(cmdVerb)
	fn, found := cmds[cmdVerb]
	if !found {
		return api.Message{}, errUnsupportedCommand
	}
	return fn()
}
