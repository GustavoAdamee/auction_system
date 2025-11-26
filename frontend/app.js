const API_BASE_URL = 'http://localhost:8081/api';

let eventSource = null;
let currentClientId = null;
let currentAuctionId = null;

function showAlert(message, type = 'info') {
    const alertDiv = document.createElement('div');
    alertDiv.className = `alert alert-${type}`;
    alertDiv.textContent = message;
    const container = document.querySelector('.container');
    container.insertBefore(alertDiv, container.firstChild);
    setTimeout(() => alertDiv.remove(), 5000);
}

function getClientId() {
    return document.getElementById('clientId').value || 'CLIENT-1';
}

async function createAuction() {
    const id = document.getElementById('newAuctionId').value;
    const description = document.getElementById('newAuctionDescription').value;
    const startTime = document.getElementById('newAuctionStart').value;
    const endTime = document.getElementById('newAuctionEnd').value;
    
    if (!id || !description || !startTime || !endTime) {
        showAlert('Preencha todos os campos!', 'error');
        return;
    }
    
    const startDate = new Date(startTime);
    const endDate = new Date(endTime);
    
    const auctionData = {
        id: id,
        description: description,
        start_time: startDate.toISOString(),
        end_time: endDate.toISOString()
    };
    
    try {
        const response = await fetch(`${API_BASE_URL}/auctions`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(auctionData)
        });
        
        if (response.ok) {
            const data = await response.json();
            showAlert(`Leilão ${data.id} criado com sucesso!`, 'success');
            loadActiveAuctions();
            document.getElementById('newAuctionId').value = '';
            document.getElementById('newAuctionDescription').value = '';
        } else {
            const error = await response.json().catch(() => ({ error: 'Falha ao criar leilão' }));
            showAlert(`Erro: ${error.error}`, 'error');
        }
    } catch (error) {
        showAlert(`Erro de conexão: ${error.message}`, 'error');
    }
}

async function loadActiveAuctions() {
    try {
        const response = await fetch(`${API_BASE_URL}/auctions`);
        
        if (response.ok) {
            const data = await response.json();
            let auctionsList = [];
            if (data === null || data === undefined) {
                auctionsList = [];
            } else if (Array.isArray(data)) {
                auctionsList = data;
            } else {
                auctionsList = [data];
            }
            
            displayAuctions(auctionsList);
            updateAuctionSelects(auctionsList);
        } else {
            showAlert('Erro ao carregar leilões', 'error');
            displayAuctions([]);
            updateAuctionSelects([]);
        }
    } catch (error) {
        showAlert(`Erro de conexão: ${error.message}`, 'error');
        displayAuctions([]);
        updateAuctionSelects([]);
    }
}

function displayAuctions(auctions) {
    const listDiv = document.getElementById('auctionsList');
    
    if (!auctions || !Array.isArray(auctions) || auctions.length === 0) {
        listDiv.innerHTML = '<p style="text-align: center; color: #999; padding: 20px;">Nenhum leilão ativo no momento</p>';
        return;
    }
    
    listDiv.innerHTML = auctions.map(auction => `
        <div class="auction-card">
            <h3>${auction.description || 'Sem descrição'}</h3>
            <p><strong>ID:</strong> ${auction.id || 'N/A'}</p>
            <p><strong>Status:</strong> <span class="status ${auction.status || 'idle'}">${auction.status || 'idle'}</span></p>
            <p><strong>Início:</strong> ${auction.start_time ? new Date(auction.start_time).toLocaleString('pt-BR') : 'N/A'}</p>
            <p><strong>Fim:</strong> ${auction.end_time ? new Date(auction.end_time).toLocaleString('pt-BR') : 'N/A'}</p>
        </div>
    `).join('');
}

function updateAuctionSelects(auctions) {
    const bidSelect = document.getElementById('bidAuctionId');
    const sseSelect = document.getElementById('sseAuctionId');
    
    if (!auctions || !Array.isArray(auctions)) {
        bidSelect.innerHTML = '<option value="">Selecione um leilão...</option>';
        sseSelect.innerHTML = '<option value="">Selecione um leilão...</option>';
        return;
    }
    
    const options = auctions.map(a => 
        `<option value="${a.id || ''}">${a.id || 'N/A'} - ${a.description || 'Sem descrição'}</option>`
    ).join('');
    
    bidSelect.innerHTML = '<option value="">Selecione um leilão...</option>' + options;
    sseSelect.innerHTML = '<option value="">Selecione um leilão...</option>' + options;
}

async function submitBid() {
    const auctionId = document.getElementById('bidAuctionId').value;
    const bidValue = document.getElementById('bidValue').value;
    const clientId = getClientId();
    
    if (!auctionId || !bidValue) {
        showAlert('Selecione um leilão e informe o valor do lance!', 'error');
        return;
    }
    
    const bidData = {
        client_id: clientId,
        bid: bidValue.toString()
    };
    
    try {
        const response = await fetch(`${API_BASE_URL}/auctions/${auctionId}/bids`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(bidData)
        });
        
        if (response.ok) {
            showAlert('Lance enviado com sucesso!', 'success');
            document.getElementById('bidValue').value = '';
        } else {
            const error = await response.json().catch(() => ({ error: 'Falha ao enviar lance' }));
            showAlert(`Erro: ${error.error}`, 'error');
        }
    } catch (error) {
        showAlert(`Erro de conexão: ${error.message}`, 'error');
    }
}

async function registerInterest() {
    const auctionId = document.getElementById('sseAuctionId').value;
    const clientId = getClientId();
    
    if (!auctionId) {
        showAlert('Selecione um leilão!', 'error');
        return;
    }
    
    try {
        const response = await fetch(`${API_BASE_URL}/auctions/${auctionId}/register-interest`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ client_id: clientId })
        });
        
        if (response.ok) {
            showAlert('Interesse registrado! Agora clique em "Conectar SSE" para receber notificações.', 'success');
        } else {
            const error = await response.json().catch(() => ({ error: 'Falha ao registrar interesse' }));
            showAlert(`Erro: ${error.error}`, 'error');
        }
    } catch (error) {
        showAlert(`Erro de conexão: ${error.message}`, 'error');
    }
}

async function cancelInterest() {
    const auctionId = document.getElementById('sseAuctionId').value;
    const clientId = getClientId();
    
    if (!auctionId) {
        showAlert('Selecione um leilão!', 'error');
        return;
    }
    
    try {
        const response = await fetch(`${API_BASE_URL}/auctions/${auctionId}/cancel-interest`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ client_id: clientId })
        });
        
        if (response.ok) {
            if (eventSource && currentAuctionId === auctionId) {
                disconnectSSE();
            }
            showAlert('Interesse cancelado com sucesso!', 'success');
        } else {
            const error = await response.json().catch(() => ({ error: 'Falha ao cancelar interesse' }));
            showAlert(`Erro: ${error.error}`, 'error');
        }
    } catch (error) {
        showAlert(`Erro de conexão: ${error.message}`, 'error');
    }
}

function connectSSE() {
    const auctionId = document.getElementById('sseAuctionId').value;
    const clientId = getClientId();
    
    if (!auctionId) {
        showAlert('Selecione um leilão!', 'error');
        return;
    }
    
    if (eventSource) {
        eventSource.close();
    }
    
    currentClientId = clientId;
    currentAuctionId = auctionId;
    
    const url = `${API_BASE_URL}/auctions/${auctionId}/events?client_id=${clientId}`;
    eventSource = new EventSource(url);
    
    eventSource.addEventListener('new_bid', handleNewBid);
    eventSource.addEventListener('invalid_bid', handleInvalidBid);
    eventSource.addEventListener('auction_winner', handleAuctionWinner);
    eventSource.addEventListener('payment_link', handlePaymentLink);
    eventSource.addEventListener('payment_status', handlePaymentStatus);
    
    eventSource.onopen = function() {
        updateSSEStatus(true);
        showAlert('Conectado ao SSE!', 'success');
    };
    
    eventSource.onerror = function() {
        updateSSEStatus(false);
        showAlert('Erro na conexão SSE', 'error');
    };
}

function disconnectSSE() {
    if (eventSource) {
        eventSource.close();
        eventSource = null;
        updateSSEStatus(false);
        showAlert('Desconectado do SSE', 'info');
    }
}

function updateSSEStatus(connected) {
    const statusDiv = document.getElementById('sseStatus');
    if (connected) {
        statusDiv.className = 'sse-status connected';
        statusDiv.textContent = 'Conectado';
    } else {
        statusDiv.className = 'sse-status disconnected';
        statusDiv.textContent = 'Desconectado';
    }
}

function handleNewBid(event) {
    try {
        const data = JSON.parse(event.data);
        const eventData = typeof data.data === 'string' ? JSON.parse(data.data) : data.data;
        addEventToLog('new_bid', 'Novo Lance Válido', eventData);
    } catch (error) {
        console.error('Erro ao processar new_bid:', error);
    }
}

function handleInvalidBid(event) {
    try {
        const data = JSON.parse(event.data);
        const eventData = typeof data.data === 'string' ? JSON.parse(data.data) : data.data;
        addEventToLog('invalid_bid', 'Lance Inválido', eventData);
    } catch (error) {
        console.error('Erro ao processar invalid_bid:', error);
    }
}

function handleAuctionWinner(event) {
    try {
        const data = JSON.parse(event.data);
        const eventData = typeof data.data === 'string' ? JSON.parse(data.data) : data.data;
        addEventToLog('auction_winner', 'Vencedor do Leilão', eventData);
    } catch (error) {
        console.error('Erro ao processar auction_winner:', error);
    }
}

function handlePaymentLink(event) {
    try {
        const data = JSON.parse(event.data);
        const eventData = typeof data.data === 'string' ? JSON.parse(data.data) : data.data;
        addEventToLog('payment_link', 'Link de Pagamento', eventData);
        if (eventData.payment_link) {
            showAlert(`Link de pagamento disponível para ${eventData.auction_id}`, 'info');
        }
    } catch (error) {
        console.error('Erro ao processar payment_link:', error);
    }
}

function handlePaymentStatus(event) {
    try {
        const data = JSON.parse(event.data);
        const eventData = typeof data.data === 'string' ? JSON.parse(data.data) : data.data;
        addEventToLog('payment_status', 'Status do Pagamento', eventData);
        if (eventData.status) {
            const statusText = eventData.status === 'approved' ? 'aprovado' : 'recusado';
            showAlert(`Pagamento ${statusText} para ${eventData.auction_id}`, eventData.status === 'approved' ? 'success' : 'error');
        }
    } catch (error) {
        console.error('Erro ao processar payment_status:', error);
    }
}

function addEventToLog(type, title, data) {
    const logDiv = document.getElementById('eventsLog');
    const eventDiv = document.createElement('div');
    eventDiv.className = `event ${type}`;
    
    const auctionId = data.auction_id ? ` (Leilão: ${data.auction_id})` : '';
    
    let eventHtml = `
        <div class="event-type">${title}${auctionId}</div>
        <div class="event-time">${new Date().toLocaleTimeString('pt-BR')}</div>
        <div class="event-data">
    `;
    
    if (type === 'new_bid') {
        eventHtml += `Lance: R$ ${data.bid} - Cliente: ${data.client_id}`;
    } else if (type === 'invalid_bid') {
        eventHtml += `Lance inválido: R$ ${data.bid} - Cliente: ${data.client_id}`;
    } else if (type === 'auction_winner') {
        eventHtml += `Vencedor: ${data.client_id} - Lance: R$ ${data.bid}`;
    } else if (type === 'payment_link') {
        if (data.payment_link) {
            eventHtml += `Link: <a href="${data.payment_link}" target="_blank">Abrir Pagamento</a>`;
        } else {
            eventHtml += 'Link de pagamento disponível';
        }
    } else if (type === 'payment_status') {
        eventHtml += `Resultado: ${data.status === 'approved' ? 'Aprovado' : 'Recusado'} (Transação: ${data.transaction_id})`;
    }
    
    eventHtml += '</div>';
    eventDiv.innerHTML = eventHtml;
    logDiv.insertBefore(eventDiv, logDiv.firstChild);
}

document.addEventListener('DOMContentLoaded', function() {
    const now = new Date();
    const startTime = new Date(now.getTime() + 10000);
    const endTime = new Date(now.getTime() + 120000);
    
    document.getElementById('newAuctionStart').value = startTime.toISOString().slice(0, 16);
    document.getElementById('newAuctionEnd').value = endTime.toISOString().slice(0, 16);
    
    loadActiveAuctions();
});
