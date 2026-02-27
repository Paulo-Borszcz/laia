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

// --- UpdateTicketStatus ---

type UpdateTicketStatus struct {
	glpi         *glpi.Client
	sessionToken string
	userID       int
}

func NewUpdateTicketStatus(g *glpi.Client, token string, userID int) *UpdateTicketStatus {
	return &UpdateTicketStatus{glpi: g, sessionToken: token, userID: userID}
}

func (t *UpdateTicketStatus) Name() string { return "update_ticket_status" }
func (t *UpdateTicketStatus) Description() string {
	return "Atualiza o status de um chamado. Status: 1=Novo, 2=Atribuído, 3=Planejado, 4=Pendente, 5=Solucionado, 6=Fechado"
}
func (t *UpdateTicketStatus) Parameters() *ai.ParamSchema {
	return &ai.ParamSchema{
		Type: "object",
		Properties: map[string]*ai.ParamSchema{
			"ticket_id": {Type: "integer", Description: "ID do chamado"},
			"status":    {Type: "integer", Description: "Novo status (5=Solucionado, 6=Fechado)"},
		},
		Required: []string{"ticket_id", "status"},
	}
}

func (t *UpdateTicketStatus) Execute(_ context.Context, args map[string]any) (map[string]any, error) {
	ticketID, err := intArg(args, "ticket_id")
	if err != nil {
		return nil, err
	}
	status, err := intArg(args, "status")
	if err != nil {
		return nil, err
	}

	err = t.glpi.UpdateTicket(t.sessionToken, ticketID, glpi.UpdateTicketInput{Status: status})
	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar chamado: %w", err)
	}
	return map[string]any{
		"mensagem": fmt.Sprintf("Chamado #%d atualizado para status: %s", ticketID, ticketStatusLabel(status)),
	}, nil
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
var _ ai.Tool = (*UpdateTicketStatus)(nil)
var _ ai.Tool = (*AddFollowup)(nil)
var _ ai.Tool = (*GetFollowups)(nil)

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
