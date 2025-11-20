package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/seba3011/tarea-3/common"
)

// ===============================================
// 🎯 MODIFICACIONES PARA USAR LAS IPs REALES
// ===============================================

// Lista de Nodos Conocidos (IP:Puerto de cada servidor)
var KnownNodes = []string{
	"10.10.31.76:9081", // CORRECCIÓN: Puerto 8081 + 1000
	"10.10.31.77:9082", // CORRECCIÓN: Puerto 8082 + 1000
	"10.10.31.78:9083", // CORRECCIÓN: Puerto 8083 + 1000
}

const MaxRetries = 3

var reader = bufio.NewReader(os.Stdin)

func main() {
	// ... (Resto del código main se mantiene igual)
	fmt.Println("=======================================")
	fmt.Println("   Sistema de Inventario Distribuido   ")
	fmt.Println("=======================================")

	for {
		fmt.Println("\n--- MENÚ PRINCIPAL ---")
		fmt.Println("1. Revisar Inventario [cite: 87]")
		fmt.Println("2. Modificar Inventario (SET_QTY / SET_PRICE) [cite: 88]")
		fmt.Println("3. Salir")
		fmt.Print("Seleccione una opción: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		option, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println("Opción no válida. Por favor, ingrese un número.")
			continue
		}

		switch option {
		case 1:
			handleReadInventory()
		case 2:
			handleModifyInventory()
		case 3:
			fmt.Println("Saliendo del programa.")
			return
		default:
			fmt.Println("Opción no reconocida.")
		}
	}
}

// sendRequestWithDiscovery: Itera sobre los KnownNodes para encontrar uno activo 
// y luego realiza el descubrimiento del Primario si el contacto inicial es un Secundario.
func sendRequestWithDiscovery(req common.ClientRequest) (*common.ClientResponse, error) {
	// 1. Iterar sobre todos los nodos conocidos para encontrar el primer nodo disponible.
	for _, initialAddress := range KnownNodes {
		resp, err := attemptRequest(req, initialAddress)
		
		// Si la solicitud tuvo éxito con un nodo inicial, procedemos con el descubrimiento/operación.
		if err == nil {
			// El nodo inicial está activo. Ahora intentamos hasta MaxRetries.
			return resolvePrimary(req, initialAddress, resp)
		}
		// Si hay error (fallo de conexión), intentamos con el siguiente nodo en la lista.
	}

	// Si ninguno de los 3 nodos responde.
	return nil, fmt.Errorf("No es posible llevar a cabo la operación: Ninguno de los 3 nodos responde al cliente ")
}

// Función auxiliar para enviar una petición a una dirección específica.
func attemptRequest(req common.ClientRequest, targetAddress string) (*common.ClientResponse, error) {
    url := fmt.Sprintf("http://%s/client_request", targetAddress)
    
    reqBody, _ := json.Marshal(req)
    
    client := &http.Client{Timeout: 3 * time.Second} // Reducir el timeout para detección de fallos de nodo
    resp, err := client.Post(url, "application/json", bytes.NewBuffer(reqBody))
    
    if err != nil {
        return nil, fmt.Errorf("error al contactar nodo en %s: %v", url, err)
    }
    defer resp.Body.Close()
    
    bodyBytes, _ := io.ReadAll(resp.Body)
    var clientResp common.ClientResponse
    if err := json.Unmarshal(bodyBytes, &clientResp); err != nil {
        return nil, fmt.Errorf("error decodificando respuesta del servidor: %v", err)
    }
    
    return &clientResp, nil
}


// Función que maneja la lógica de redirección y reintento.
func resolvePrimary(req common.ClientRequest, initialAddress string, initialResp *common.ClientResponse) (*common.ClientResponse, error) {
    currentAddress := initialAddress
    currentResponse := initialResp

    for i := 0; i < MaxRetries; i++ {
        // Si el nodo actual es el primario, la operación fue completada o la respuesta contiene el inventario.
        if currentResponse.IsPrimary {
            return currentResponse, nil
        }

        // Si el nodo actual es secundario, debe indicar el ID del primario actual[cite: 94].
        if currentResponse.PrimaryID > 0 {
            // Asumiendo la convención de IDs 1, 2, 3 que mapean a los índices de KnownNodes.
            // Esto asume que el ID coincide con el índice + 1 en la lista KnownNodes.
            primaryIndex := currentResponse.PrimaryID - 1 
            
            if primaryIndex >= 0 && primaryIndex < len(KnownNodes) {
                currentAddress = KnownNodes[primaryIndex]
                fmt.Printf("Redireccionando al Primario (ID %d) en %s...\n", currentResponse.PrimaryID, currentAddress)
                time.Sleep(500 * time.Millisecond) 
                
                // Intentar la solicitud en la nueva dirección (Primario descubierto)
                resp, err := attemptRequest(req, currentAddress)
                if err != nil {
                     // Si el primario descubierto cae, se intenta con otro nodo conocido en el siguiente loop.
                    return nil, fmt.Errorf("El primario (ID %d) no respondió. Reintentando descubrimiento", currentResponse.PrimaryID)
                }
                currentResponse = resp
            } else {
                 return nil, fmt.Errorf("Redirección fallida: ID Primario (%d) inválido", currentResponse.PrimaryID)
            }
        } else {
            // El nodo no es Primario y no conoce al Primario actual (o está en medio de una elección).
            // La lógica de reintento debería contactar a un nodo diferente.
            return nil, fmt.Errorf("El nodo contactado no conoce al Primario actual. Reintento manual necesario")
        }
    }
    
    return nil, fmt.Errorf("Fallo al contactar al Primario después de %d intentos de redirección.", MaxRetries)
}

func handleReadInventory() {
	req := common.ClientRequest{Type: common.OpReadInventory}
	
	// Utilizamos la nueva función sendRequestWithDiscovery
	resp, err := sendRequestWithDiscovery(req)
	
	if err != nil {
		fmt.Printf("ERROR: Fallo en la lectura del inventario: %v\n", err)
		return
	}
	
	fmt.Println("\n--- INVENTARIO ACTUAL ---")
	if resp.Inventory == nil || len(resp.Inventory) == 0 {
		fmt.Println("Inventario vacío.")
		return
	}
	
	// Se debe desplegar la lista de ítems del inventario, con nombre, cantidad y precio[cite: 87].
	fmt.Printf("%-15s %-10s %-10s\n", "ITEM", "CANTIDAD", "PRECIO")
	fmt.Println("---------------------------------")
	for name, item := range resp.Inventory {
		fmt.Printf("%-15s %-10d %-10d\n", name, item.Quantity, item.Price)
	}
}

func handleModifyInventory() {
	fmt.Println("\n--- MODIFICAR INVENTARIO ---")
	
	// ... (La lógica de entrada de usuario para obtener opType, itemName, y newValue se mantiene igual)
	fmt.Print("¿Desea modificar (1) Cantidad (SET_QTY) o (2) Precio (SET_PRICE)?: ")
	opInput, _ := reader.ReadString('\n')
	opInput = strings.TrimSpace(opInput)
	
	var opType common.MessageType
	if opInput == "1" {
		opType = common.OpSetQuantity
	} else if opInput == "2" {
		opType = common.OpSetPrice
	} else {
		fmt.Println("Opción no válida.")
		return
	}

	fmt.Print("Ingrese el nombre del Ítem a modificar (Ej: LAPICES): ")
	itemName, _ := reader.ReadString('\n')
	itemName = strings.TrimSpace(strings.ToUpper(itemName))
	
	fmt.Print("Ingrese el nuevo valor: ")
	valInput, _ := reader.ReadString('\n')
	valInput = strings.TrimSpace(valInput)
	newValue, err := strconv.Atoi(valInput)
	if err != nil || newValue < 0 {
		fmt.Println("Valor no válido. Debe ser un número entero no negativo.")
		return
	}
	
	req := common.ClientRequest{
		Type:     opType,
		ItemName: itemName,
		NewValue: newValue,
	}
	
	// Utilizamos la nueva función sendRequestWithDiscovery
	resp, err := sendRequestWithDiscovery(req)
	
	if err != nil {
		fmt.Printf("ERROR: Fallo en la modificación del inventario: %v\n", err)
		return
	}
	
	if resp.Success {
		fmt.Printf("✅ Éxito: Operación %s de %s completada. Nueva secuencia: %d\n", opType, itemName, resp.SeqNumber)
	} else {
		fmt.Printf("❌ Fallo: El Primario rechazó la operación. Mensaje: %s\n", resp.Error)
	}
}