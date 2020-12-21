package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/tmax-cloud/cicd-operator/internal/utils"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cicdv1 "github.com/tmax-cloud/cicd-operator/api/v1"
)

var webhookPath = fmt.Sprintf("/webhook/{%s}/{%s}", paramKeyNamespace, paramKeyConfigName)

type webhookHandler struct {
	k8sClient client.Client
}

func (h *webhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	ns, nsExist := vars[paramKeyNamespace]
	configName, configNameExist := vars[paramKeyConfigName]

	if !nsExist || !configNameExist {
		_ = utils.RespondError(w, http.StatusBadRequest, fmt.Sprintf("path is not in form of '%s'", webhookPath))
		log.Info("Bad request for path", "path", r.RequestURI)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		_ = utils.RespondError(w, http.StatusInternalServerError, "cannot read webhook body")
		log.Error(err, "cannot read webhook body")
		return
	}

	config := &cicdv1.IntegrationConfig{}
	if err := h.k8sClient.Get(context.TODO(), types.NamespacedName{Name: configName, Namespace: ns}, config); err != nil {
		_ = utils.RespondError(w, http.StatusBadRequest, fmt.Sprintf("cannot get IntegrationConfig %s/%s", ns, configName))
		log.Info("Bad request for path", "path", r.RequestURI)
		return
	}

	gitCli, err := utils.GetGitCli(config)
	if err != nil {
		_ = utils.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert webhook
	wh, err := gitCli.ParseWebhook(config, r.Header, body)
	if err != nil {
		_ = utils.RespondError(w, http.StatusInternalServerError, "cannot parse webhook body")
		log.Error(err, "")
		return
	}

	// Call plugin functions
	plugins := GetPlugins(wh.EventType)
	for _, p := range plugins {
		if err := p.Handle(wh, config); err != nil {
			log.Error(err, "")
		}
	}
}