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

var KnownNodes = []string{
	"10.10.31.76:9081",
	"10.10.31.77:9082", 
	"10.10.31.78:9083", 
}

const MaxRetries = 3

var reader = bufio.NewReader(os.Stdin)

func main() {
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

func sendRequestWithDiscovery(req common.ClientRequest) (*common.ClientResponse, error) {

	for _, initialAddress := range KnownNodes {
		resp, err := attemptRequest(req, initialAddress)

		if err == nil {

			return resolvePrimary(req, initialAddress, resp)
		}

	}

	return nil, fmt.Errorf("No es posible llevar a cabo la operación: Ninguno de los 3 nodos responde al cliente ")
}

func attemptRequest(req common.ClientRequest, targetAddress string) (*common.ClientResponse, error) {
    url := fmt.Sprintf("http://%s/client_request", targetAddress)
    
    reqBody, _ := json.Marshal(req)
    
    client := &http.Client{Timeout: 3 * time.Second} 
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

func resolvePrimary(req common.ClientRequest, initialAddress string, initialResp *common.ClientResponse) (*common.ClientResponse, error) {
    currentAddress := initialAddress
    currentResponse := initialResp

    for i := 0; i < MaxRetries; i++ {

        if currentResponse.IsPrimary {
            return currentResponse, nil
        }

        if currentResponse.PrimaryID > 0 {

            primaryIndex := currentResponse.PrimaryID - 1 
            
            if primaryIndex >= 0 && primaryIndex < len(KnownNodes) {
                currentAddress = KnownNodes[primaryIndex]
                fmt.Printf("Redireccionando al Primario (ID %d) en %s...\n", currentResponse.PrimaryID, currentAddress)
                time.Sleep(500 * time.Millisecond) 
                
                resp, err := attemptRequest(req, currentAddress)
                if err != nil {
                    return nil, fmt.Errorf("El primario (ID %d) no respondió. Reintentando descubrimiento", currentResponse.PrimaryID)
                }
                currentResponse = resp
            } else {
                 return nil, fmt.Errorf("Redirección fallida: ID Primario (%d) inválido", currentResponse.PrimaryID)
            }
        } else {

            return nil, fmt.Errorf("El nodo contactado no conoce al Primario actual. Reintento manual necesario")
        }
    }
    
    return nil, fmt.Errorf("Fallo al contactar al Primario después de %d intentos de redirección.", MaxRetries)
}

func handleReadInventory() {
	req := common.ClientRequest{Type: common.OpReadInventory}

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

	fmt.Printf("%-15s %-10s %-10s\n", "ITEM", "CANTIDAD", "PRECIO")
	fmt.Println("---------------------------------")
	for name, item := range resp.Inventory {
		fmt.Printf("%-15s %-10d %-10d\n", name, item.Quantity, item.Price)
	}
}

func handleModifyInventory() {
	fmt.Println("\n--- MODIFICAR INVENTARIO ---")

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

	resp, err := sendRequestWithDiscovery(req)
	
	if err != nil {
		fmt.Printf("ERROR: Fallo en la modificación del inventario: %v\n", err)
		return
	}
	
	if resp.Success {
		fmt.Printf("Éxito: Operación %s de %s completada. Nueva secuencia: %d\n", opType, itemName, resp.SeqNumber)
	} else {
		fmt.Printf("Fallo: El Primario rechazó la operación. Mensaje: %s\n", resp.Error)
	}
}