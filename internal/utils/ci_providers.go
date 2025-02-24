// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/DataDog/dd-sdk-go-testing/internal/constants"
	"github.com/mitchellh/go-homedir"
)

type providerType = func() map[string]string

var providers = map[string]providerType{
	"APPVEYOR":           extractAppveyor,
	"TF_BUILD":           extractAzurePipelines,
	"BITBUCKET_COMMIT":   extractBitbucket,
	"BUDDY":              extractBuddy,
	"BUILDKITE":          extractBuildkite,
	"CIRCLECI":           extractCircleCI,
	"GITHUB_SHA":         extractGithubActions,
	"GITLAB_CI":          extractGitlab,
	"JENKINS_URL":        extractJenkins,
	"TEAMCITY_VERSION":   extractTeamcity,
	"TRAVIS":             extractTravis,
	"BITRISE_BUILD_SLUG": extractBitrise,
}

func removeEmpty(tags map[string]string) {
	for tag, value := range tags {
		if value == "" {
			delete(tags, tag)
		}
	}
}

// GetProviderTags extracts CI information from environment variables.
func GetProviderTags() map[string]string {
	tags := map[string]string{}
	for key, provider := range providers {
		if _, ok := os.LookupEnv(key); !ok {
			continue
		}
		tags = provider()
	}

	// replace with user specific tags
	replaceWithUserSpecificTags(tags)

	// Normalize tags
	normalizeTags(tags)

	// Expand ~
	if tag, ok := tags[constants.CIWorkspacePath]; ok && tag != "" {
		homedir.Reset()
		if value, err := homedir.Expand(tag); err == nil {
			tags[constants.CIWorkspacePath] = value
		}
	}

	// remove empty values
	removeEmpty(tags)

	return tags
}

func normalizeTags(tags map[string]string) {
	if tag, ok := tags[constants.GitBranch]; ok && tag != "" {
		if strings.Contains(tag, "refs/tags") || strings.Contains(tag, "origin/tags") || strings.Contains(tag, "refs/heads/tags") {
			tags[constants.GitTag] = normalizeRef(tag)
		}
		tags[constants.GitBranch] = normalizeRef(tag)
	}
	if tag, ok := tags[constants.GitTag]; ok && tag != "" {
		tags[constants.GitTag] = normalizeRef(tag)
	}
	if tag, ok := tags[constants.GitRepositoryURL]; ok && tag != "" {
		tags[constants.GitRepositoryURL] = filterSensitiveInfo(tag)
	}
}

func replaceWithUserSpecificTags(tags map[string]string) {

	replace := func(tagName, envName string) {
		tags[tagName] = getEnvironmentVariableIfIsNotEmpty(envName, tags[tagName])
	}

	replace(constants.GitBranch, "DD_GIT_BRANCH")
	replace(constants.GitTag, "DD_GIT_TAG")
	replace(constants.GitRepositoryURL, "DD_GIT_REPOSITORY_URL")
	replace(constants.GitCommitSHA, "DD_GIT_COMMIT_SHA")
	replace(constants.GitCommitMessage, "DD_GIT_COMMIT_MESSAGE")
	replace(constants.GitCommitAuthorName, "DD_GIT_COMMIT_AUTHOR_NAME")
	replace(constants.GitCommitAuthorEmail, "DD_GIT_COMMIT_AUTHOR_EMAIL")
	replace(constants.GitCommitAuthorDate, "DD_GIT_COMMIT_AUTHOR_DATE")
	replace(constants.GitCommitCommitterName, "DD_GIT_COMMIT_COMMITTER_NAME")
	replace(constants.GitCommitCommitterEmail, "DD_GIT_COMMIT_COMMITTER_EMAIL")
	replace(constants.GitCommitCommitterDate, "DD_GIT_COMMIT_COMMITTER_DATE")
}

func getEnvironmentVariableIfIsNotEmpty(key string, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	} else {
		return defaultValue
	}
}

func normalizeRef(name string) string {
	empty := []byte("")
	refs := regexp.MustCompile("^refs/(heads/)?")
	origin := regexp.MustCompile("^origin/")
	tags := regexp.MustCompile("^tags/")
	return string(tags.ReplaceAll(origin.ReplaceAll(refs.ReplaceAll([]byte(name), empty), empty), empty)[:])
}

func filterSensitiveInfo(url string) string {
	return string(regexp.MustCompile("(https?://)[^/]*@").ReplaceAll([]byte(url), []byte("$1"))[:])
}

func lookupEnvs(keys ...string) ([]string, bool) {
	values := make([]string, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func extractAppveyor() map[string]string {
	tags := map[string]string{}
	url := fmt.Sprintf("https://ci.appveyor.com/project/%s/builds/%s", os.Getenv("APPVEYOR_REPO_NAME"), os.Getenv("APPVEYOR_BUILD_ID"))
	tags[constants.CIProviderName] = "appveyor"
	if os.Getenv("APPVEYOR_REPO_PROVIDER") == "github" {
		tags[constants.GitRepositoryURL] = fmt.Sprintf("https://github.com/%s.git", os.Getenv("APPVEYOR_REPO_NAME"))
	} else {
		tags[constants.GitRepositoryURL] = os.Getenv("APPVEYOR_REPO_NAME")
	}

	tags[constants.GitCommitSHA] = os.Getenv("APPVEYOR_REPO_COMMIT")
	tags[constants.GitBranch] = firstEnv("APPVEYOR_PULL_REQUEST_HEAD_REPO_BRANCH", "APPVEYOR_REPO_BRANCH")
	tags[constants.GitTag] = os.Getenv("APPVEYOR_REPO_TAG_NAME")

	tags[constants.CIWorkspacePath] = os.Getenv("APPVEYOR_BUILD_FOLDER")
	tags[constants.CIPipelineID] = os.Getenv("APPVEYOR_BUILD_ID")
	tags[constants.CIPipelineName] = os.Getenv("APPVEYOR_REPO_NAME")
	tags[constants.CIPipelineNumber] = os.Getenv("APPVEYOR_BUILD_NUMBER")
	tags[constants.CIPipelineURL] = url
	tags[constants.CIJobURL] = url
	tags[constants.GitCommitMessage] = fmt.Sprintf("%s\n%s", os.Getenv("APPVEYOR_REPO_COMMIT_MESSAGE"), os.Getenv("APPVEYOR_REPO_COMMIT_MESSAGE_EXTENDED"))
	tags[constants.GitCommitAuthorName] = os.Getenv("APPVEYOR_REPO_COMMIT_AUTHOR")
	tags[constants.GitCommitAuthorEmail] = os.Getenv("APPVEYOR_REPO_COMMIT_AUTHOR_EMAIL")
	return tags
}

func extractAzurePipelines() map[string]string {
	tags := map[string]string{}
	baseURL := fmt.Sprintf("%s%s/_build/results?buildId=%s", os.Getenv("SYSTEM_TEAMFOUNDATIONSERVERURI"), os.Getenv("SYSTEM_TEAMPROJECTID"), os.Getenv("BUILD_BUILDID"))
	pipelineURL := baseURL
	jobURL := fmt.Sprintf("%s&view=logs&j=%s&t=%s", baseURL, os.Getenv("SYSTEM_JOBID"), os.Getenv("SYSTEM_TASKINSTANCEID"))
	branchOrTag := firstEnv("SYSTEM_PULLREQUEST_SOURCEBRANCH", "BUILD_SOURCEBRANCH", "BUILD_SOURCEBRANCHNAME")
	branch := ""
	tag := ""
	if strings.Contains(branchOrTag, "tags/") {
		tag = branchOrTag
	} else {
		branch = branchOrTag
	}
	tags[constants.CIProviderName] = "azurepipelines"
	tags[constants.CIWorkspacePath] = os.Getenv("BUILD_SOURCESDIRECTORY")

	tags[constants.CIPipelineID] = os.Getenv("BUILD_BUILDID")
	tags[constants.CIPipelineName] = os.Getenv("BUILD_DEFINITIONNAME")
	tags[constants.CIPipelineNumber] = os.Getenv("BUILD_BUILDID")
	tags[constants.CIPipelineURL] = pipelineURL

	tags[constants.CIStageName] = os.Getenv("SYSTEM_STAGEDISPLAYNAME")

	tags[constants.CIJobName] = os.Getenv("SYSTEM_JOBDISPLAYNAME")
	tags[constants.CIJobURL] = jobURL

	tags[constants.GitRepositoryURL] = firstEnv("SYSTEM_PULLREQUEST_SOURCEREPOSITORYURI", "BUILD_REPOSITORY_URI")
	tags[constants.GitCommitSHA] = firstEnv("SYSTEM_PULLREQUEST_SOURCECOMMITID", "BUILD_SOURCEVERSION")
	tags[constants.GitBranch] = branch
	tags[constants.GitTag] = tag
	tags[constants.GitCommitMessage] = os.Getenv("BUILD_SOURCEVERSIONMESSAGE")
	tags[constants.GitCommitAuthorName] = os.Getenv("BUILD_REQUESTEDFORID")
	tags[constants.GitCommitAuthorEmail] = os.Getenv("BUILD_REQUESTEDFOREMAIL")

	envVarsMap := map[string]string{
		"SYSTEM_TEAMPROJECTID": os.Getenv("SYSTEM_TEAMPROJECTID"),
		"BUILD_BUILDID":        os.Getenv("BUILD_BUILDID"),
		"SYSTEM_JOBID":         os.Getenv("SYSTEM_JOBID"),
	}
	removeEmpty(envVarsMap)
	jsonString, err := json.Marshal(envVarsMap)
	if err == nil {
		tags[constants.CIEnvVars] = string(jsonString)
	}

	return tags
}

func extractBitrise() map[string]string {
	tags := map[string]string{}
	tags[constants.CIProviderName] = "bitrise"
	tags[constants.GitRepositoryURL] = os.Getenv("GIT_REPOSITORY_URL")
	tags[constants.GitCommitSHA] = firstEnv("BITRISE_GIT_COMMIT", "GIT_CLONE_COMMIT_HASH")
	tags[constants.GitBranch] = firstEnv("BITRISEIO_GIT_BRANCH_DEST", "BITRISE_GIT_BRANCH")
	tags[constants.GitTag] = os.Getenv("BITRISE_GIT_TAG")
	tags[constants.CIWorkspacePath] = os.Getenv("BITRISE_SOURCE_DIR")
	tags[constants.CIPipelineID] = os.Getenv("BITRISE_BUILD_SLUG")
	tags[constants.CIPipelineName] = os.Getenv("BITRISE_TRIGGERED_WORKFLOW_ID")
	tags[constants.CIPipelineNumber] = os.Getenv("BITRISE_BUILD_NUMBER")
	tags[constants.CIPipelineURL] = os.Getenv("BITRISE_BUILD_URL")
	tags[constants.GitCommitMessage] = os.Getenv("BITRISE_GIT_MESSAGE")
	return tags
}

func extractBitbucket() map[string]string {
	tags := map[string]string{}
	url := fmt.Sprintf("https://bitbucket.org/%s/addon/pipelines/home#!/results/%s", os.Getenv("BITBUCKET_REPO_FULL_NAME"), os.Getenv("BITBUCKET_BUILD_NUMBER"))
	tags[constants.CIProviderName] = "bitbucket"
	tags[constants.GitRepositoryURL] = os.Getenv("BITBUCKET_GIT_SSH_ORIGIN")
	tags[constants.GitCommitSHA] = os.Getenv("BITBUCKET_COMMIT")
	tags[constants.GitBranch] = os.Getenv("BITBUCKET_BRANCH")
	tags[constants.GitTag] = os.Getenv("BITBUCKET_TAG")
	tags[constants.CIWorkspacePath] = os.Getenv("BITBUCKET_CLONE_DIR")
	tags[constants.CIPipelineID] = strings.Trim(os.Getenv("BITBUCKET_PIPELINE_UUID"), "{}")
	tags[constants.CIPipelineNumber] = os.Getenv("BITBUCKET_BUILD_NUMBER")
	tags[constants.CIPipelineName] = os.Getenv("BITBUCKET_REPO_FULL_NAME")
	tags[constants.CIPipelineURL] = url
	tags[constants.CIJobURL] = url
	return tags
}

func extractBuddy() map[string]string {
	tags := map[string]string{}
	tags[constants.CIProviderName] = "buddy"
	tags[constants.CIPipelineID] = fmt.Sprintf("%s/%s", os.Getenv("BUDDY_PIPELINE_ID"), os.Getenv("BUDDY_EXECUTION_ID"))
	tags[constants.CIPipelineName] = os.Getenv("BUDDY_PIPELINE_NAME")
	tags[constants.CIPipelineNumber] = os.Getenv("BUDDY_EXECUTION_ID")
	tags[constants.CIPipelineURL] = os.Getenv("BUDDY_EXECUTION_URL")
	tags[constants.GitCommitSHA] = os.Getenv("BUDDY_EXECUTION_REVISION")
	tags[constants.GitRepositoryURL] = os.Getenv("BUDDY_SCM_URL")
	tags[constants.GitBranch] = os.Getenv("BUDDY_EXECUTION_BRANCH")
	tags[constants.GitTag] = os.Getenv("BUDDY_EXECUTION_TAG")
	tags[constants.GitCommitMessage] = os.Getenv("BUDDY_EXECUTION_REVISION_MESSAGE")
	tags[constants.GitCommitCommitterName] = os.Getenv("BUDDY_EXECUTION_REVISION_COMMITTER_NAME")
	tags[constants.GitCommitCommitterEmail] = os.Getenv("BUDDY_EXECUTION_REVISION_COMMITTER_EMAIL")
	return tags
}

func extractBuildkite() map[string]string {
	tags := map[string]string{}
	tags[constants.GitBranch] = os.Getenv("BUILDKITE_BRANCH")
	tags[constants.GitCommitSHA] = os.Getenv("BUILDKITE_COMMIT")
	tags[constants.GitRepositoryURL] = os.Getenv("BUILDKITE_REPO")
	tags[constants.GitTag] = os.Getenv("BUILDKITE_TAG")
	tags[constants.CIPipelineID] = os.Getenv("BUILDKITE_BUILD_ID")
	tags[constants.CIPipelineName] = os.Getenv("BUILDKITE_PIPELINE_SLUG")
	tags[constants.CIPipelineNumber] = os.Getenv("BUILDKITE_BUILD_NUMBER")
	tags[constants.CIPipelineURL] = os.Getenv("BUILDKITE_BUILD_URL")
	tags[constants.CIJobURL] = fmt.Sprintf("%s#%s", os.Getenv("BUILDKITE_BUILD_URL"), os.Getenv("BUILDKITE_JOB_ID"))
	tags[constants.CIProviderName] = "buildkite"
	tags[constants.CIWorkspacePath] = os.Getenv("BUILDKITE_BUILD_CHECKOUT_PATH")
	tags[constants.GitCommitMessage] = os.Getenv("BUILDKITE_MESSAGE")
	tags[constants.GitCommitAuthorName] = os.Getenv("BUILDKITE_BUILD_AUTHOR")
	tags[constants.GitCommitAuthorEmail] = os.Getenv("BUILDKITE_BUILD_AUTHOR_EMAIL")

	envVarsMap := map[string]string{
		"BUILDKITE_BUILD_ID": os.Getenv("BUILDKITE_BUILD_ID"),
		"BUILDKITE_JOB_ID":   os.Getenv("BUILDKITE_JOB_ID"),
	}
	removeEmpty(envVarsMap)
	jsonString, err := json.Marshal(envVarsMap)
	if err == nil {
		tags[constants.CIEnvVars] = string(jsonString)
	}

	return tags
}

func extractCircleCI() map[string]string {
	tags := map[string]string{}
	tags[constants.CIProviderName] = "circleci"
	tags[constants.GitRepositoryURL] = os.Getenv("CIRCLE_REPOSITORY_URL")
	tags[constants.GitCommitSHA] = os.Getenv("CIRCLE_SHA1")
	tags[constants.GitTag] = os.Getenv("CIRCLE_TAG")
	tags[constants.GitBranch] = os.Getenv("CIRCLE_BRANCH")
	tags[constants.CIWorkspacePath] = os.Getenv("CIRCLE_WORKING_DIRECTORY")
	tags[constants.CIPipelineID] = os.Getenv("CIRCLE_WORKFLOW_ID")
	tags[constants.CIPipelineName] = os.Getenv("CIRCLE_PROJECT_REPONAME")
	tags[constants.CIPipelineNumber] = os.Getenv("CIRCLE_BUILD_NUM")
	tags[constants.CIPipelineURL] = fmt.Sprintf("https://app.circleci.com/pipelines/workflows/%s", os.Getenv("CIRCLE_WORKFLOW_ID"))
	tags[constants.CIJobName] = os.Getenv("CIRCLE_JOB")
	tags[constants.CIJobURL] = os.Getenv("CIRCLE_BUILD_URL")

	envVarsMap := map[string]string{
		"CIRCLE_BUILD_NUM":   os.Getenv("CIRCLE_BUILD_NUM"),
		"CIRCLE_WORKFLOW_ID": os.Getenv("CIRCLE_WORKFLOW_ID"),
	}
	removeEmpty(envVarsMap)
	jsonString, err := json.Marshal(envVarsMap)
	if err == nil {
		tags[constants.CIEnvVars] = string(jsonString)
	}

	return tags
}

func extractGithubActions() map[string]string {
	tags := map[string]string{}
	branchOrTag := firstEnv("GITHUB_HEAD_REF", "GITHUB_REF")
	tag := ""
	branch := ""
	if strings.Contains(branchOrTag, "tags/") {
		tag = branchOrTag
	} else {
		branch = branchOrTag
	}

	serverUrl := os.Getenv("GITHUB_SERVER_URL")
	if serverUrl == "" {
		serverUrl = "https://github.com"
	}
	serverUrl = strings.TrimSuffix(serverUrl, "/")

	rawRepository := fmt.Sprintf("%s/%s", serverUrl, os.Getenv("GITHUB_REPOSITORY"))
	pipelineId := os.Getenv("GITHUB_RUN_ID")
	commitSha := os.Getenv("GITHUB_SHA")

	tags[constants.CIProviderName] = "github"
	tags[constants.GitRepositoryURL] = rawRepository + ".git"
	tags[constants.GitCommitSHA] = commitSha
	tags[constants.GitBranch] = branch
	tags[constants.GitTag] = tag
	tags[constants.CIWorkspacePath] = os.Getenv("GITHUB_WORKSPACE")
	tags[constants.CIPipelineID] = pipelineId
	tags[constants.CIPipelineNumber] = os.Getenv("GITHUB_RUN_NUMBER")
	tags[constants.CIPipelineName] = os.Getenv("GITHUB_WORKFLOW")
	tags[constants.CIJobURL] = fmt.Sprintf("%s/commit/%s/checks", rawRepository, commitSha)
	tags[constants.CIJobName] = os.Getenv("GITHUB_JOB")

	attempts := os.Getenv("GITHUB_RUN_ATTEMPT")
	if attempts == "" {
		tags[constants.CIPipelineURL] = fmt.Sprintf("%s/actions/runs/%s", rawRepository, pipelineId)
	} else {
		tags[constants.CIPipelineURL] = fmt.Sprintf("%s/actions/runs/%s/attempts/%s", rawRepository, pipelineId, attempts)
	}

	envVarsMap := map[string]string{
		"GITHUB_SERVER_URL":  os.Getenv("GITHUB_SERVER_URL"),
		"GITHUB_REPOSITORY":  os.Getenv("GITHUB_REPOSITORY"),
		"GITHUB_RUN_ID":      os.Getenv("GITHUB_RUN_ID"),
		"GITHUB_RUN_ATTEMPT": os.Getenv("GITHUB_RUN_ATTEMPT"),
	}
	removeEmpty(envVarsMap)
	jsonString, err := json.Marshal(envVarsMap)
	if err == nil {
		tags[constants.CIEnvVars] = string(jsonString)
	}

	return tags
}

func extractGitlab() map[string]string {
	tags := map[string]string{}
	url := os.Getenv("CI_PIPELINE_URL")
	url = string(regexp.MustCompile("/-/pipelines/").ReplaceAll([]byte(url), []byte("/pipelines/"))[:])
	url = strings.ReplaceAll(url, "/-/pipelines/", "/pipelines/")

	tags[constants.CIProviderName] = "gitlab"
	tags[constants.GitRepositoryURL] = os.Getenv("CI_REPOSITORY_URL")
	tags[constants.GitCommitSHA] = os.Getenv("CI_COMMIT_SHA")
	tags[constants.GitBranch] = firstEnv("CI_COMMIT_BRANCH", "CI_COMMIT_REF_NAME")
	tags[constants.GitTag] = os.Getenv("CI_COMMIT_TAG")
	tags[constants.CIWorkspacePath] = os.Getenv("CI_PROJECT_DIR")
	tags[constants.CIPipelineID] = os.Getenv("CI_PIPELINE_ID")
	tags[constants.CIPipelineName] = os.Getenv("CI_PROJECT_PATH")
	tags[constants.CIPipelineNumber] = os.Getenv("CI_PIPELINE_IID")
	tags[constants.CIPipelineURL] = url
	tags[constants.CIJobURL] = os.Getenv("CI_JOB_URL")
	tags[constants.CIJobName] = os.Getenv("CI_JOB_NAME")
	tags[constants.CIStageName] = os.Getenv("CI_JOB_STAGE")
	tags[constants.GitCommitMessage] = os.Getenv("CI_COMMIT_MESSAGE")

	author := os.Getenv("CI_COMMIT_AUTHOR")
	authorArray := strings.FieldsFunc(author, func(s rune) bool {
		return s == '<' || s == '>'
	})
	tags[constants.GitCommitAuthorName] = strings.TrimSpace(authorArray[0])
	tags[constants.GitCommitAuthorEmail] = strings.TrimSpace(authorArray[1])
	tags[constants.GitCommitAuthorDate] = os.Getenv("CI_COMMIT_TIMESTAMP")

	envVarsMap := map[string]string{
		"CI_PROJECT_URL": os.Getenv("CI_PROJECT_URL"),
		"CI_PIPELINE_ID": os.Getenv("CI_PIPELINE_ID"),
		"CI_JOB_ID":      os.Getenv("CI_JOB_ID"),
	}
	removeEmpty(envVarsMap)
	jsonString, err := json.Marshal(envVarsMap)
	if err == nil {
		tags[constants.CIEnvVars] = string(jsonString)
	}

	return tags
}

func extractJenkins() map[string]string {
	tags := map[string]string{}
	tags[constants.CIProviderName] = "jenkins"
	tags[constants.GitRepositoryURL] = firstEnv("GIT_URL", "GIT_URL_1")
	tags[constants.GitCommitSHA] = os.Getenv("GIT_COMMIT")

	branchOrTag := os.Getenv("GIT_BRANCH")
	empty := []byte("")
	name, hasName := os.LookupEnv("JOB_NAME")

	if strings.Contains(branchOrTag, "tags/") {
		tags[constants.GitTag] = branchOrTag
	} else {
		tags[constants.GitBranch] = branchOrTag
		// remove branch for job name
		removeBranch := regexp.MustCompile(fmt.Sprintf("/%s", normalizeRef(branchOrTag)))
		name = string(removeBranch.ReplaceAll([]byte(name), empty))
	}

	if hasName {
		removeVars := regexp.MustCompile("/[^/]+=[^/]*")
		name = string(removeVars.ReplaceAll([]byte(name), empty))
	}

	tags[constants.CIWorkspacePath] = os.Getenv("WORKSPACE")
	tags[constants.CIPipelineID] = os.Getenv("BUILD_TAG")
	tags[constants.CIPipelineNumber] = os.Getenv("BUILD_NUMBER")
	tags[constants.CIPipelineName] = name
	tags[constants.CIPipelineURL] = os.Getenv("BUILD_URL")

	envVarsMap := map[string]string{
		"DD_CUSTOM_TRACE_ID": os.Getenv("DD_CUSTOM_TRACE_ID"),
	}
	removeEmpty(envVarsMap)
	jsonString, err := json.Marshal(envVarsMap)
	if err == nil {
		tags[constants.CIEnvVars] = string(jsonString)
	}

	return tags
}

func extractTeamcity() map[string]string {
	tags := map[string]string{}
	tags[constants.CIProviderName] = "teamcity"
	tags[constants.GitRepositoryURL] = os.Getenv("BUILD_VCS_URL")
	tags[constants.GitCommitSHA] = os.Getenv("BUILD_VCS_NUMBER")
	tags[constants.CIWorkspacePath] = os.Getenv("BUILD_CHECKOUTDIR")
	tags[constants.CIPipelineID] = os.Getenv("BUILD_ID")
	tags[constants.CIPipelineNumber] = os.Getenv("BUILD_NUMBER")
	tags[constants.CIPipelineURL] = fmt.Sprintf("%s/viewLog.html?buildId=%s", os.Getenv("SERVER_URL"), os.Getenv("BUILD_ID"))
	return tags
}

func extractTravis() map[string]string {
	tags := map[string]string{}
	prSlug := os.Getenv("TRAVIS_PULL_REQUEST_SLUG")
	repoSlug := prSlug
	if strings.TrimSpace(repoSlug) == "" {
		repoSlug = os.Getenv("TRAVIS_REPO_SLUG")
	}
	tags[constants.CIProviderName] = "travisci"
	tags[constants.GitRepositoryURL] = fmt.Sprintf("https://github.com/%s.git", repoSlug)
	tags[constants.GitCommitSHA] = os.Getenv("TRAVIS_COMMIT")
	tags[constants.GitTag] = os.Getenv("TRAVIS_TAG")
	tags[constants.GitBranch] = firstEnv("TRAVIS_PULL_REQUEST_BRANCH", "TRAVIS_BRANCH")
	tags[constants.CIWorkspacePath] = os.Getenv("TRAVIS_BUILD_DIR")
	tags[constants.CIPipelineID] = os.Getenv("TRAVIS_BUILD_ID")
	tags[constants.CIPipelineNumber] = os.Getenv("TRAVIS_BUILD_NUMBER")
	tags[constants.CIPipelineName] = repoSlug
	tags[constants.CIPipelineURL] = os.Getenv("TRAVIS_BUILD_WEB_URL")
	tags[constants.CIJobURL] = os.Getenv("TRAVIS_JOB_WEB_URL")
	tags[constants.GitCommitMessage] = os.Getenv("TRAVIS_COMMIT_MESSAGE")
	return tags
}
