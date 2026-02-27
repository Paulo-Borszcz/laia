package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	return `Lista os chamados do usuario atual no Nexus/GLPI.
Quando usar: quando o usuario quiser ver seus proprios chamados sem filtros complexos. Ex: "meus chamados", "meu ultimo chamado".
Parametros opcionais: status (filtra por estado), limit (quantidade maxima de resultados).
Retorna: lista com id, nome, status e data de cada chamado.
Prefira search_tickets_advanced quando houver filtros por texto, periodo, urgencia ou tecnico.`
}
func (t *ListMyTickets) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"status": {
				Type:        "string",
				Description: "Filtrar por status: aberto, pendente, solucionado, fechado, todos. Default: todos",
				Enum:        []string{"aberto", "pendente", "solucionado", "fechado", "todos"},
			},
			"limit": {
				Type:        "integer",
				Description: "Quantidade maxima de chamados retornados (1-50). Default: 20. Use limit=1 para 'meu ultimo chamado'.",
			},
		},
	}
}

func (t *ListMyTickets) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	tickets, err := t.glpi.GetMyTickets(t.sessionToken)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar chamados: %w", err)
	}

	statusFilter := optionalStringArg(args, "status")
	limit := optionalIntArg(args, "limit")
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	allowedStatuses := mapStatusToGLPI(statusFilter)

	var filtered []map[string]any
	for _, tk := range tickets {
		if len(allowedStatuses) > 0 && !intInSlice(tk.Status, allowedStatuses) {
			continue
		}
		filtered = append(filtered, map[string]any{
			"id":     tk.ID,
			"nome":   tk.Name,
			"status": ticketStatusLabel(tk.Status),
			"data":   tk.DateCreated,
		})
	}

	totalSemFiltro := len(tickets)
	if limit < len(filtered) {
		filtered = filtered[:limit]
	}

	result := map[string]any{"total": len(filtered), "chamados": filtered}
	if statusFilter != "" && statusFilter != "todos" {
		result["total_sem_filtro"] = totalSemFiltro
	}
	return result, nil
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
	return `Busca chamados por palavra-chave, status, periodo, urgencia ou tecnico.
Quando usar: sempre que o usuario quiser encontrar chamados por algum criterio. Ex: "chamados de VPN", "chamados abertos", "chamados do mes".
O campo 'query' busca no titulo E descricao simultaneamente.
Se nenhum criterio for informado, pedira esclarecimento ao usuario.
Prefira esta ferramenta sobre list_my_tickets quando houver qualquer filtro de busca.
Retorna: lista com id, titulo, status, datas, urgencia, categoria, tecnico e solicitante.`
}
func (t *SearchTicketsAdvanced) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"query": {
				Type:        "string",
				Description: "Busca no titulo E descricao simultaneamente. Ex: 'VPN', 'impressora', 'email'",
			},
			"status": {
				Type:        "string",
				Description: "Filtrar por status: aberto (novo+atribuido+planejado), pendente, solucionado, fechado, todos",
				Enum:        []string{"aberto", "pendente", "solucionado", "fechado", "todos"},
			},
			"period": {
				Type:        "string",
				Description: "Periodo: hoje, semana, mes, ano, ou intervalo YYYY-MM-DD..YYYY-MM-DD",
			},
			"urgency": {
				Type:        "string",
				Description: "Filtrar por urgencia: muito_baixa, baixa, media, alta, muito_alta",
				Enum:        []string{"muito_baixa", "baixa", "media", "alta", "muito_alta"},
			},
			"assigned_to": {
				Type:        "string",
				Description: "Nome parcial do tecnico atribuido. Ex: 'João', 'Silva'",
			},
			"requester": {
				Type:        "string",
				Description: "Nome parcial do solicitante. Ex: 'Maria', 'Santos'",
			},
		},
	}
}

func (t *SearchTicketsAdvanced) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	query := optionalStringArg(args, "query")
	status := optionalStringArg(args, "status")
	period := optionalStringArg(args, "period")
	urgency := optionalStringArg(args, "urgency")
	assignedTo := optionalStringArg(args, "assigned_to")
	requester := optionalStringArg(args, "requester")

	if query == "" && status == "" && period == "" && urgency == "" && assignedTo == "" && requester == "" {
		return clarification(
			"O que voce gostaria de buscar? Informe pelo menos um criterio.",
			[]string{"texto (ex: VPN)", "status (aberto/pendente)", "periodo (hoje/semana/mes)", "urgencia", "tecnico atribuido"},
			"Use search_tickets_advanced com pelo menos um parametro preenchido.",
		), nil
	}

	// GLPI search uses criteria groups: top-level criteria linked with AND,
	// sub-criteria inside a group linked with OR.
	// Reference: nexus_apirest.md — criteria[N][criteria][M] for sub-groups.
	criteria := map[string]string{}
	idx := 0

	// addTopCriteria adds a single top-level AND criterion.
	addTopCriteria := func(field, searchType, value string) {
		if idx > 0 {
			criteria[fmt.Sprintf("criteria[%d][link]", idx)] = "AND"
		}
		criteria[fmt.Sprintf("criteria[%d][field]", idx)] = field
		criteria[fmt.Sprintf("criteria[%d][searchtype]", idx)] = searchType
		criteria[fmt.Sprintf("criteria[%d][value]", idx)] = value
		idx++
	}

	// addORGroup adds a top-level AND group with multiple OR sub-criteria.
	addORGroup := func(field, searchType string, values []string) {
		if idx > 0 {
			criteria[fmt.Sprintf("criteria[%d][link]", idx)] = "AND"
		}
		for j, v := range values {
			prefix := fmt.Sprintf("criteria[%d][criteria][%d]", idx, j)
			if j > 0 {
				criteria[prefix+"[link]"] = "OR"
			}
			criteria[prefix+"[field]"] = field
			criteria[prefix+"[searchtype]"] = searchType
			criteria[prefix+"[value]"] = v
		}
		idx++
	}

	// query: title OR content (sub-group)
	if query != "" {
		addORGroup("1", "contains", []string{query, query})
		// Override second sub-criterion to search field 21 (content)
		prefix := fmt.Sprintf("criteria[%d][criteria][1]", idx-1)
		criteria[prefix+"[field]"] = "21"
	}

	// status: "aberto" = 1 OR 2 OR 3 (sub-group)
	if status != "" && status != "todos" {
		statusCodes := mapStatusToGLPI(status)
		vals := make([]string, len(statusCodes))
		for i, c := range statusCodes {
			vals[i] = fmt.Sprintf("%d", c)
		}
		addORGroup("12", "equals", vals)
	}

	// period: date range with AND
	if period != "" {
		dateFrom, dateTo := parsePeriod(period)
		if dateFrom != "" {
			addTopCriteria("15", "morethan", dateFrom)
		}
		if dateTo != "" {
			addTopCriteria("15", "lessthan", dateTo)
		}
	}

	if urgency != "" {
		code := mapUrgencyToGLPI(urgency)
		if code > 0 {
			addTopCriteria("10", "equals", fmt.Sprintf("%d", code))
		}
	}

	if assignedTo != "" {
		addTopCriteria("5", "contains", assignedTo)
	}
	if requester != "" {
		addTopCriteria("4", "contains", requester)
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

// --- search helpers ---

// mapStatusToGLPI converts friendly status names to GLPI status codes.
// "aberto" groups Novo(1), Atribuído(2), Planejado(3).
func mapStatusToGLPI(status string) []int {
	switch status {
	case "aberto":
		return []int{1, 2, 3}
	case "pendente":
		return []int{4}
	case "solucionado":
		return []int{5}
	case "fechado":
		return []int{6}
	default:
		return nil
	}
}

// parsePeriod converts friendly period names or date ranges to (from, to) date strings.
func parsePeriod(period string) (string, string) {
	now := time.Now()
	today := now.Format("2006-01-02")

	switch period {
	case "hoje":
		return today, today
	case "semana":
		weekAgo := now.AddDate(0, 0, -7).Format("2006-01-02")
		return weekAgo, today
	case "mes":
		monthAgo := now.AddDate(0, -1, 0).Format("2006-01-02")
		return monthAgo, today
	case "ano":
		yearAgo := now.AddDate(-1, 0, 0).Format("2006-01-02")
		return yearAgo, today
	default:
		// YYYY-MM-DD..YYYY-MM-DD
		if parts := strings.SplitN(period, "..", 2); len(parts) == 2 {
			return parts[0], parts[1]
		}
		return "", ""
	}
}

func mapUrgencyToGLPI(urgency string) int {
	switch urgency {
	case "muito_baixa":
		return 1
	case "baixa":
		return 2
	case "media":
		return 3
	case "alta":
		return 4
	case "muito_alta":
		return 5
	default:
		return 0
	}
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
