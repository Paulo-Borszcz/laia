package tools

import (
	"context"
	"fmt"

	"github.com/lojasmm/laia/internal/ai"
	"github.com/lojasmm/laia/internal/glpi"
)

// --- ListMyTickets ---

type ListMyTickets struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewListMyTickets(g *glpi.Client, token string) *ListMyTickets {
	return &ListMyTickets{glpi: g, sessionToken: token}
}

func (t *ListMyTickets) Name() string { return "list_my_tickets" }
func (t *ListMyTickets) Description() string {
	return "Lista todos os chamados do usuário atual no Nexus/GLPI"
}
func (t *ListMyTickets) Parameters() *ai.ParamSchema { return nil }

func (t *ListMyTickets) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	tickets, err := t.glpi.GetMyTickets(t.sessionToken)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar chamados: %w", err)
	}

	items := make([]map[string]any, len(tickets))
	for i, tk := range tickets {
		items[i] = map[string]any{
			"id":     tk.ID,
			"nome":   tk.Name,
			"status": ticketStatusLabel(tk.Status),
			"data":   tk.DateCreated,
		}
	}
	return map[string]any{"total": len(tickets), "chamados": items}, nil
}

// --- GetTicket ---

type GetTicket struct {
	glpi         *glpi.Client
	sessionToken string
	userID       int
}

func NewGetTicket(g *glpi.Client, token string, userID int) *GetTicket {
	return &GetTicket{glpi: g, sessionToken: token, userID: userID}
}

func (t *GetTicket) Name() string { return "get_ticket" }
func (t *GetTicket) Description() string {
	return "Retorna detalhes de um chamado específico pelo ID"
}
func (t *GetTicket) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"ticket_id": {Type: "integer", Description: "ID do chamado"},
		},
		Required: []string{"ticket_id"},
	}
}

func (t *GetTicket) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	ticketID, err := intArg(args, "ticket_id")
	if err != nil {
		return nil, err
	}

	ticket, err := t.glpi.GetTicket(t.sessionToken, ticketID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar chamado: %w", err)
	}

	return map[string]any{
		"id":            ticket.ID,
		"titulo":        ticket.Name,
		"descricao":     ticket.Content,
		"status":        ticketStatusLabel(ticket.Status),
		"urgencia":      urgencyLabel(ticket.Urgency),
		"prioridade":    priorityLabel(ticket.Priority),
		"categoria":     ticket.ITILCategoriesID,
		"criado_em":     ticket.DateCreated,
		"atualizado_em": ticket.DateMod,
	}, nil
}

// --- CreateTicket ---

type CreateTicket struct {
	glpi   *glpi.Client
	userID int
}

func NewCreateTicket(g *glpi.Client, userID int) *CreateTicket {
	return &CreateTicket{glpi: g, userID: userID}
}

func (t *CreateTicket) Name() string { return "create_ticket" }
func (t *CreateTicket) Description() string {
	return "Cria um novo chamado no Nexus/GLPI. Use somente após confirmação do usuário."
}
func (t *CreateTicket) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"title":         {Type: "string", Description: "Título do chamado"},
			"description":   {Type: "string", Description: "Descrição detalhada do problema"},
			"category_id":   {Type: "integer", Description: "ID da categoria ITIL (obrigatório, obtido via get_department_categories)"},
			"department_id": {Type: "integer", Description: "ID do departamento/formulário (obtido via get_departments)"},
			"urgency":       {Type: "integer", Description: "Urgência: 1=Muito baixa, 2=Baixa, 3=Média, 4=Alta, 5=Muito alta"},
		},
		Required: []string{"title", "description", "category_id", "department_id"},
	}
}

func (t *CreateTicket) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	title, _ := stringArg(args, "title")
	description, _ := stringArg(args, "description")
	if title == "" || description == "" {
		return nil, fmt.Errorf("título e descrição são obrigatórios")
	}

	catID, err := intArg(args, "category_id")
	if err != nil || catID <= 0 {
		return nil, fmt.Errorf("category_id é obrigatório — use get_department_categories para obter o ID")
	}

	formID, _ := intArg(args, "department_id")

	// Usa admin session pois usuários self-service não têm permissão
	// para criar tickets diretamente via API (só via FormCreator na web).
	adminSession, err := t.glpi.AdminSession()
	if err != nil {
		return nil, fmt.Errorf("erro ao criar sessão admin: %w", err)
	}
	defer t.glpi.KillSession(adminSession)

	input := glpi.CreateTicketInput{
		Name:             title,
		Content:          description,
		Type:             1, // Incidente
		ITILCategoriesID: catID,
		UsersIDRequester: t.userID,
	}
	if urgency, err := intArg(args, "urgency"); err == nil && urgency >= 1 && urgency <= 5 {
		input.Urgency = urgency
	}

	// Aplica as mesmas regras de actors do FormCreator (observadores, grupos atribuídos)
	if formID > 0 {
		applyFormActors(t.glpi, adminSession, formID, t.userID, &input)
	}

	id, err := t.glpi.CreateTicket(adminSession, input)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar chamado: %w", err)
	}
	return map[string]any{"id": id, "mensagem": fmt.Sprintf("Chamado #%d criado com sucesso", id)}, nil
}

// applyFormActors reads the FormCreator target ticket config and applies the
// same actors (assigned groups/users, observers) that the web form would apply.
func applyFormActors(g *glpi.Client, session string, formID, requesterID int, input *glpi.CreateTicketInput) {
	targets, err := g.GetTargetTickets(session, formID)
	if err != nil || len(targets) == 0 {
		return
	}

	actors, err := g.GetTargetActors(session, targets[0].ID)
	if err != nil {
		return
	}

	for _, a := range actors {
		if a.ActorValue == 0 && a.ActorType == 1 {
			continue // Creator = requester, already set
		}
		// FormCreator roles: 1=Requester, 2=Observer, 3=Assigned
		switch a.ActorRole {
		case 2: // Observer
			switch a.ActorType {
			case 3: // Specific user
				input.UsersIDObserver = append(input.UsersIDObserver, a.ActorValue)
			case 5: // Specific group
				input.GroupsIDObserver = append(input.GroupsIDObserver, a.ActorValue)
			}
		case 3: // Assigned
			switch a.ActorType {
			case 3: // Specific user
				input.UsersIDAssign = append(input.UsersIDAssign, a.ActorValue)
			case 5: // Specific group
				input.GroupsIDAssign = append(input.GroupsIDAssign, a.ActorValue)
			}
		}
	}
}

// --- UpdateTicket ---

type UpdateTicket struct {
	glpi         *glpi.Client
	sessionToken string
	userID       int
}

func NewUpdateTicket(g *glpi.Client, token string, userID int) *UpdateTicket {
	return &UpdateTicket{glpi: g, sessionToken: token, userID: userID}
}

func (t *UpdateTicket) Name() string { return "update_ticket" }
func (t *UpdateTicket) Description() string {
	return "Atualiza campos de um chamado: status, urgência, título, descrição ou categoria. Passe apenas os campos que deseja alterar."
}
func (t *UpdateTicket) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"ticket_id":   {Type: "integer", Description: "ID do chamado"},
			"status":      {Type: "integer", Description: "Novo status: 1=Novo, 2=Atribuído, 3=Planejado, 4=Pendente, 5=Solucionado, 6=Fechado"},
			"urgency":     {Type: "integer", Description: "Urgência: 1=Muito baixa, 2=Baixa, 3=Média, 4=Alta, 5=Muito alta"},
			"title":       {Type: "string", Description: "Novo título do chamado"},
			"description": {Type: "string", Description: "Nova descrição do chamado"},
			"category_id": {Type: "integer", Description: "Nova categoria ITIL"},
		},
		Required: []string{"ticket_id"},
	}
}

func (t *UpdateTicket) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	ticketID, err := intArg(args, "ticket_id")
	if err != nil {
		return nil, err
	}

	input := glpi.UpdateTicketInput{}
	changes := []string{}

	if s, err := intArg(args, "status"); err == nil {
		input.Status = s
		changes = append(changes, "status → "+ticketStatusLabel(s))
	}
	if u, err := intArg(args, "urgency"); err == nil && u >= 1 && u <= 5 {
		input.Urgency = u
		changes = append(changes, "urgência → "+urgencyLabel(u))
	}
	if title, _ := args["title"].(string); title != "" {
		input.Name = title
		changes = append(changes, "título")
	}
	if desc, _ := args["description"].(string); desc != "" {
		input.Content = desc
		changes = append(changes, "descrição")
	}
	if catID, err := intArg(args, "category_id"); err == nil {
		input.ITILCategoriesID = catID
		changes = append(changes, "categoria")
	}

	if len(changes) == 0 {
		return nil, fmt.Errorf("nenhum campo para atualizar")
	}

	err = t.glpi.UpdateTicket(t.sessionToken, ticketID, input)
	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar chamado: %w", err)
	}
	return map[string]any{
		"mensagem":   fmt.Sprintf("Chamado #%d atualizado", ticketID),
		"alteracoes": changes,
	}, nil
}

// --- SearchTicketsAdvanced ---

type SearchTicketsAdvanced struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewSearchTicketsAdvanced(g *glpi.Client, token string) *SearchTicketsAdvanced {
	return &SearchTicketsAdvanced{glpi: g, sessionToken: token}
}

func (t *SearchTicketsAdvanced) Name() string { return "search_tickets_advanced" }
func (t *SearchTicketsAdvanced) Description() string {
	return "Busca chamados com filtros avançados. Combine qualquer conjunto de filtros: status, texto, urgência, técnico, solicitante, observador, datas."
}
func (t *SearchTicketsAdvanced) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"status":          {Type: "integer", Description: "Filtrar por status: 1=Novo, 2=Atribuído, 3=Planejado, 4=Pendente, 5=Solucionado, 6=Fechado"},
			"text":            {Type: "string", Description: "Buscar no título do chamado (contém)"},
			"content":         {Type: "string", Description: "Buscar no conteúdo/descrição do chamado (contém)"},
			"urgency":         {Type: "integer", Description: "Filtrar por urgência: 1-5"},
			"assigned_to":     {Type: "string", Description: "Nome do técnico atribuído (contém)"},
			"requester":       {Type: "string", Description: "Nome do solicitante (contém)"},
			"observer":        {Type: "string", Description: "Nome do observador (contém)"},
			"date_from":       {Type: "string", Description: "Data de abertura a partir de (formato: YYYY-MM-DD)"},
			"date_to":         {Type: "string", Description: "Data de abertura até (formato: YYYY-MM-DD)"},
			"close_date_from": {Type: "string", Description: "Data de fechamento a partir de (formato: YYYY-MM-DD)"},
			"close_date_to":   {Type: "string", Description: "Data de fechamento até (formato: YYYY-MM-DD)"},
		},
	}
}

func (t *SearchTicketsAdvanced) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	criteria := map[string]string{}
	idx := 0

	addCriteria := func(field, searchType, value string) {
		if idx > 0 {
			criteria[fmt.Sprintf("criteria[%d][link]", idx)] = "AND"
		}
		criteria[fmt.Sprintf("criteria[%d][field]", idx)] = field
		criteria[fmt.Sprintf("criteria[%d][searchtype]", idx)] = searchType
		criteria[fmt.Sprintf("criteria[%d][value]", idx)] = value
		idx++
	}

	if text, _ := args["text"].(string); text != "" {
		addCriteria("1", "contains", text) // Title
	}
	if content, _ := args["content"].(string); content != "" {
		addCriteria("21", "contains", content) // Description body
	}
	if status, err := intArg(args, "status"); err == nil {
		addCriteria("12", "equals", fmt.Sprintf("%d", status))
	}
	if urgency, err := intArg(args, "urgency"); err == nil {
		addCriteria("10", "equals", fmt.Sprintf("%d", urgency))
	}
	if v, _ := args["assigned_to"].(string); v != "" {
		addCriteria("5", "contains", v) // Assigned technician
	}
	if v, _ := args["requester"].(string); v != "" {
		addCriteria("4", "contains", v) // Requester
	}
	if v, _ := args["observer"].(string); v != "" {
		addCriteria("66", "contains", v) // Observer
	}
	if v, _ := args["date_from"].(string); v != "" {
		addCriteria("15", "morethan", v) // Opening date >=
	}
	if v, _ := args["date_to"].(string); v != "" {
		addCriteria("15", "lessthan", v) // Opening date <=
	}
	if v, _ := args["close_date_from"].(string); v != "" {
		addCriteria("16", "morethan", v) // Closing date >=
	}
	if v, _ := args["close_date_to"].(string); v != "" {
		addCriteria("16", "lessthan", v) // Closing date <=
	}

	result, err := t.glpi.AdvancedSearchTickets(t.sessionToken, criteria)
	if err != nil {
		return nil, fmt.Errorf("erro na busca: %w", err)
	}

	items := make([]map[string]any, len(result.Data))
	for i, d := range result.Data {
		items[i] = map[string]any{
			"id":              d["2"],
			"titulo":          d["1"],
			"status":          d["12"],
			"data_abertura":   d["15"],
			"data_fechamento": d["16"],
			"urgencia":        d["10"],
			"prioridade":      d["3"],
			"categoria":       d["7"],
			"tecnico":         d["5"],
			"solicitante":     d["4"],
		}
	}
	return map[string]any{"total": result.TotalCount, "chamados": items}, nil
}

// --- GetTicketTasks ---

type GetTicketTasks struct {
	glpi         *glpi.Client
	sessionToken string
	userID       int
}

func NewGetTicketTasks(g *glpi.Client, token string, userID int) *GetTicketTasks {
	return &GetTicketTasks{glpi: g, sessionToken: token, userID: userID}
}

func (t *GetTicketTasks) Name() string { return "get_ticket_tasks" }
func (t *GetTicketTasks) Description() string {
	return "Lista as tarefas/atividades de um chamado"
}
func (t *GetTicketTasks) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"ticket_id": {Type: "integer", Description: "ID do chamado"},
		},
		Required: []string{"ticket_id"},
	}
}

func (t *GetTicketTasks) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	ticketID, err := intArg(args, "ticket_id")
	if err != nil {
		return nil, err
	}

	tasks, err := t.glpi.GetTicketTasks(t.sessionToken, ticketID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar tarefas: %w", err)
	}

	items := make([]map[string]any, len(tasks))
	for i, task := range tasks {
		items[i] = map[string]any{
			"id":       task.ID,
			"conteudo": task.Content,
			"estado":   taskStateLabel(task.State),
			"progresso": task.PercentDone,
			"data":     task.DateCreated,
		}
	}
	return map[string]any{"total": len(tasks), "tarefas": items}, nil
}

// --- AddTicketTask ---

type AddTicketTask struct {
	glpi         *glpi.Client
	sessionToken string
	userID       int
}

func NewAddTicketTask(g *glpi.Client, token string, userID int) *AddTicketTask {
	return &AddTicketTask{glpi: g, sessionToken: token, userID: userID}
}

func (t *AddTicketTask) Name() string { return "add_ticket_task" }
func (t *AddTicketTask) Description() string {
	return "Cria uma tarefa em um chamado existente"
}
func (t *AddTicketTask) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"ticket_id": {Type: "integer", Description: "ID do chamado"},
			"content":   {Type: "string", Description: "Descrição da tarefa"},
			"state":     {Type: "integer", Description: "Estado: 1=A fazer, 2=Em andamento, 3=Feito. Padrão: 1"},
		},
		Required: []string{"ticket_id", "content"},
	}
}

func (t *AddTicketTask) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	ticketID, err := intArg(args, "ticket_id")
	if err != nil {
		return nil, err
	}
	content, _ := stringArg(args, "content")
	if content == "" {
		return nil, fmt.Errorf("conteúdo da tarefa é obrigatório")
	}
	state := 1
	if s, err := intArg(args, "state"); err == nil && s >= 1 && s <= 3 {
		state = s
	}

	id, err := t.glpi.AddTicketTask(t.sessionToken, ticketID, content, state)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar tarefa: %w", err)
	}
	return map[string]any{
		"id":       id,
		"mensagem": fmt.Sprintf("Tarefa criada no chamado #%d", ticketID),
	}, nil
}

// --- ApproveTicket ---

type ApproveTicket struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewApproveTicket(g *glpi.Client, token string) *ApproveTicket {
	return &ApproveTicket{glpi: g, sessionToken: token}
}

func (t *ApproveTicket) Name() string { return "approve_ticket" }
func (t *ApproveTicket) Description() string {
	return "Aprova ou recusa uma validação/aprovação pendente em um chamado"
}
func (t *ApproveTicket) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"ticket_id": {Type: "integer", Description: "ID do chamado"},
			"approve":   {Type: "string", Description: "Aprovar ou recusar", Enum: []string{"sim", "nao"}},
			"comment":   {Type: "string", Description: "Comentário da aprovação/recusa (opcional)"},
		},
		Required: []string{"ticket_id", "approve"},
	}
}

func (t *ApproveTicket) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	ticketID, err := intArg(args, "ticket_id")
	if err != nil {
		return nil, err
	}
	approveStr, _ := stringArg(args, "approve")
	approve := approveStr == "sim"
	comment, _ := args["comment"].(string)

	validations, err := t.glpi.GetTicketValidations(t.sessionToken, ticketID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar validações: %w", err)
	}

	// Encontrar validação pendente (status=2 = Waiting)
	var pendingID int
	for _, v := range validations {
		if v.Status == 2 {
			pendingID = v.ID
			break
		}
	}
	if pendingID == 0 {
		return nil, fmt.Errorf("nenhuma aprovação pendente no chamado #%d", ticketID)
	}

	err = t.glpi.RespondTicketValidation(t.sessionToken, pendingID, approve, comment)
	if err != nil {
		return nil, fmt.Errorf("erro ao responder validação: %w", err)
	}

	action := "aprovado"
	if !approve {
		action = "recusado"
	}
	return map[string]any{
		"mensagem": fmt.Sprintf("Chamado #%d %s", ticketID, action),
	}, nil
}

// --- RateTicket ---

type RateTicket struct {
	glpi         *glpi.Client
	sessionToken string
}

func NewRateTicket(g *glpi.Client, token string) *RateTicket {
	return &RateTicket{glpi: g, sessionToken: token}
}

func (t *RateTicket) Name() string { return "rate_ticket" }
func (t *RateTicket) Description() string {
	return "Envia avaliação de satisfação para um chamado solucionado/fechado"
}
func (t *RateTicket) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"ticket_id": {Type: "integer", Description: "ID do chamado"},
			"rating":    {Type: "integer", Description: "Nota de 1 a 5 (1=Péssimo, 5=Excelente)"},
			"comment":   {Type: "string", Description: "Comentário sobre o atendimento (opcional)"},
		},
		Required: []string{"ticket_id", "rating"},
	}
}

func (t *RateTicket) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	ticketID, err := intArg(args, "ticket_id")
	if err != nil {
		return nil, err
	}
	rating, err := intArg(args, "rating")
	if err != nil {
		return nil, err
	}
	if rating < 1 || rating > 5 {
		return nil, fmt.Errorf("nota deve ser de 1 a 5")
	}
	comment, _ := args["comment"].(string)

	satisfaction, err := t.glpi.GetTicketSatisfaction(t.sessionToken, ticketID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar pesquisa de satisfação: %w", err)
	}
	if satisfaction == nil {
		return nil, fmt.Errorf("não há pesquisa de satisfação disponível para o chamado #%d", ticketID)
	}

	err = t.glpi.RateTicketSatisfaction(t.sessionToken, satisfaction.ID, rating, comment)
	if err != nil {
		return nil, fmt.Errorf("erro ao enviar avaliação: %w", err)
	}
	return map[string]any{
		"mensagem": fmt.Sprintf("Avaliação de %d estrelas enviada para o chamado #%d", rating, ticketID),
	}, nil
}

// --- GetTicketHistory ---

type GetTicketHistory struct {
	glpi         *glpi.Client
	sessionToken string
	userID       int
}

func NewGetTicketHistory(g *glpi.Client, token string, userID int) *GetTicketHistory {
	return &GetTicketHistory{glpi: g, sessionToken: token, userID: userID}
}

func (t *GetTicketHistory) Name() string { return "get_ticket_history" }
func (t *GetTicketHistory) Description() string {
	return "Mostra o histórico de alterações de um chamado (quem mudou o quê e quando)"
}
func (t *GetTicketHistory) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"ticket_id": {Type: "integer", Description: "ID do chamado"},
		},
		Required: []string{"ticket_id"},
	}
}

func (t *GetTicketHistory) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	ticketID, err := intArg(args, "ticket_id")
	if err != nil {
		return nil, err
	}

	logs, err := t.glpi.GetTicketLogs(t.sessionToken, ticketID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar histórico: %w", err)
	}

	items := make([]map[string]any, len(logs))
	for i, l := range logs {
		items[i] = map[string]any{
			"data":        l.DateMod,
			"usuario":     l.UsersName,
			"valor_antigo": l.OldValue,
			"valor_novo":  l.NewValue,
		}
	}
	return map[string]any{"total": len(logs), "historico": items}, nil
}

// --- AddFollowup ---

type AddFollowup struct {
	glpi         *glpi.Client
	sessionToken string
	userID       int
}

func NewAddFollowup(g *glpi.Client, token string, userID int) *AddFollowup {
	return &AddFollowup{glpi: g, sessionToken: token, userID: userID}
}

func (t *AddFollowup) Name() string { return "add_followup" }
func (t *AddFollowup) Description() string {
	return "Adiciona um comentário (followup) a um chamado existente"
}
func (t *AddFollowup) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"ticket_id": {Type: "integer", Description: "ID do chamado"},
			"content":   {Type: "string", Description: "Texto do comentário"},
		},
		Required: []string{"ticket_id", "content"},
	}
}

func (t *AddFollowup) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	ticketID, err := intArg(args, "ticket_id")
	if err != nil {
		return nil, err
	}
	content, _ := stringArg(args, "content")
	if content == "" {
		return nil, fmt.Errorf("conteúdo do comentário é obrigatório")
	}

	id, err := t.glpi.AddFollowup(t.sessionToken, ticketID, content)
	if err != nil {
		return nil, fmt.Errorf("erro ao adicionar comentário: %w", err)
	}
	return map[string]any{
		"id":       id,
		"mensagem": fmt.Sprintf("Comentário adicionado ao chamado #%d", ticketID),
	}, nil
}

// --- GetFollowups ---

type GetFollowups struct {
	glpi         *glpi.Client
	sessionToken string
	userID       int
}

func NewGetFollowups(g *glpi.Client, token string, userID int) *GetFollowups {
	return &GetFollowups{glpi: g, sessionToken: token, userID: userID}
}

func (t *GetFollowups) Name() string { return "get_followups" }
func (t *GetFollowups) Description() string {
	return "Lista os comentários (followups) de um chamado"
}
func (t *GetFollowups) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"ticket_id": {Type: "integer", Description: "ID do chamado"},
		},
		Required: []string{"ticket_id"},
	}
}

func (t *GetFollowups) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	ticketID, err := intArg(args, "ticket_id")
	if err != nil {
		return nil, err
	}

	followups, err := t.glpi.GetFollowups(t.sessionToken, ticketID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar comentários: %w", err)
	}

	items := make([]map[string]any, len(followups))
	for i, f := range followups {
		items[i] = map[string]any{
			"id":       f.ID,
			"conteudo": f.Content,
			"data":     f.DateCreated,
		}
	}
	return map[string]any{"total": len(followups), "comentarios": items}, nil
}

// --- helpers ---

var _ ai.Tool = (*ListMyTickets)(nil)
var _ ai.Tool = (*GetTicket)(nil)
var _ ai.Tool = (*CreateTicket)(nil)
var _ ai.Tool = (*UpdateTicket)(nil)
var _ ai.Tool = (*AddFollowup)(nil)
var _ ai.Tool = (*GetFollowups)(nil)
var _ ai.Tool = (*SearchTicketsAdvanced)(nil)
var _ ai.Tool = (*GetTicketTasks)(nil)
var _ ai.Tool = (*AddTicketTask)(nil)
var _ ai.Tool = (*ApproveTicket)(nil)
var _ ai.Tool = (*RateTicket)(nil)
var _ ai.Tool = (*GetTicketHistory)(nil)

func ticketStatusLabel(s int) string {
	switch s {
	case 1:
		return "Novo"
	case 2:
		return "Em atendimento (atribuído)"
	case 3:
		return "Em atendimento (planejado)"
	case 4:
		return "Pendente"
	case 5:
		return "Solucionado"
	case 6:
		return "Fechado"
	default:
		return fmt.Sprintf("Desconhecido (%d)", s)
	}
}

func urgencyLabel(u int) string {
	switch u {
	case 1:
		return "Muito baixa"
	case 2:
		return "Baixa"
	case 3:
		return "Média"
	case 4:
		return "Alta"
	case 5:
		return "Muito alta"
	default:
		return fmt.Sprintf("Desconhecida (%d)", u)
	}
}

func taskStateLabel(s int) string {
	switch s {
	case 1:
		return "A fazer"
	case 2:
		return "Em andamento"
	case 3:
		return "Feito"
	default:
		return fmt.Sprintf("Desconhecido (%d)", s)
	}
}

func priorityLabel(p int) string {
	switch p {
	case 1:
		return "Muito baixa"
	case 2:
		return "Baixa"
	case 3:
		return "Média"
	case 4:
		return "Alta"
	case 5:
		return "Muito alta"
	case 6:
		return "Maior"
	default:
		return fmt.Sprintf("Desconhecida (%d)", p)
	}
}
