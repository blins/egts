package main

import (
	"encoding/binary"
	"io"
	"net"
	"time"

	"github.com/kuznetsovin/egts.go"
	"github.com/satori/go.uuid"
)

const (
	egtsPcOk = 0
)

func handleRecvPkg(conn net.Conn, store Connector) {
	var (
		isPkgSave         bool
		srResultCodePkg   []byte
		serviceType       uint8
		srResponsesRecord egts.RecordDataSet
	)
	buf := make([]byte, 1024)

	if store == nil {
		logger.Errorf("Не корректная ссылка на объект хранилища")
		conn.Close()
		return
	}
	logger.Warnf("Установлено соединение с %s", conn.RemoteAddr())

	for {
	Received:
		serviceType = 0
		srResponsesRecord = nil
		srResultCodePkg = nil

		pkgLen, err := conn.Read(buf)

		connTimer := time.NewTimer(config.Srv.getEmptyConnTTL())
		switch err {
		case nil:
			connTimer.Reset(config.Srv.getEmptyConnTTL())
			logger.Debugf("Принят пакет: %X\v", buf[:pkgLen])
			break
		case io.EOF:
			<-connTimer.C
			conn.Close()
			logger.Warnf("Соединение %s закрыто по таймауту", conn.RemoteAddr())
			return
		default:
			logger.Errorf("Ошибка при получении:", err)
			conn.Close()
			return
		}

		logger.Debugf("Принят пакет: %X\v", buf)

		pkg := egts.Package{}
		receivedTimestamp := time.Now().UTC().Unix()
		resultCode, err := pkg.Decode(buf[:pkgLen])
		if err != nil {
			logger.Errorf("Не удалось расшифровать пакет: %v", err)

			resp, err := createPtResponse(&pkg, resultCode, serviceType, nil)
			if err != nil {
				logger.Errorf("Ошибка сборки ответа EGTS_PT_RESPONSE с ошибкой: %v", err)
				goto Received
			}
			_, _ = conn.Write(resp)

			goto Received
		}

		switch pkg.PacketType {
		case egts.PtAppdataPacket:
			logger.Info("Тип пакета EGTS_PT_APPDATA")

			for _, rec := range *pkg.ServicesFrameData.(*egts.ServiceDataSet) {
				exportPacket := EgtsParsePacket{
					PacketID: uint32(pkg.PacketIdentifier),
				}

				isPkgSave = false
				packetIdBytes := make([]byte, 4)

				srResponsesRecord = append(srResponsesRecord, egts.RecordData{
					SubrecordType:   egts.SrRecordResponseType,
					SubrecordLength: 3,
					SubrecordData: &egts.SrResponse{
						ConfirmedRecordNumber: rec.RecordNumber,
						RecordStatus:          egtsPcOk,
					},
				})
				serviceType = rec.SourceServiceType
				logger.Info("Тип сервиса ", serviceType)

				exportPacket.Client = rec.ObjectIdentifier

				for _, subRec := range rec.RecordDataSet {
					switch subRecData := subRec.SubrecordData.(type) {
					case *egts.SrTermIdentity:
						logger.Debugf("Разбор подзаписи EGTS_SR_TERM_IDENTITY")
						if srResultCodePkg, err = createSrResultCode(&pkg, egtsPcOk); err != nil {
							logger.Errorf("Ошибка сборки EGTS_SR_RESULT_CODE: %v", err)
						}
					case *egts.SrAuthInfo:
						logger.Debugf("Разбор подзаписи EGTS_SR_AUTH_INFO")
						if srResultCodePkg, err = createSrResultCode(&pkg, egtsPcOk); err != nil {
							logger.Errorf("Ошибка сборки EGTS_SR_RESULT_CODE: %v", err)
						}
					case *egts.SrResponse:
						logger.Debugf("Разбор подзаписи EGTS_SR_RESPONSE")
						goto Received
					case *egts.SrPosData:
						logger.Debugf("Разбор подзаписи EGTS_SR_POS_DATA")
						isPkgSave = true

						exportPacket.NavigationTimestamp = subRecData.NavigationTime.Unix()
						exportPacket.ReceivedTimestamp = receivedTimestamp
						exportPacket.Latitude = subRecData.Latitude
						exportPacket.Longitude = subRecData.Longitude
						exportPacket.Speed = subRecData.Speed
						exportPacket.Course = subRecData.Direction
						exportPacket.Guid, _ = uuid.NewV4()
					case *egts.SrExtPosData:
						logger.Debugf("Разбор подзаписи EGTS_SR_EXT_POS_DATA")
						exportPacket.Nsat = subRecData.Satellites
						exportPacket.Pdop = subRecData.PositionDilutionOfPrecision
						exportPacket.Hdop = subRecData.HorizontalDilutionOfPrecision
						exportPacket.Vdop = subRecData.VerticalDilutionOfPrecision
						exportPacket.Ns = subRecData.NavigationSystem

					case *egts.SrAdSensorsData:
						logger.Debugf("Разбор подзаписи EGTS_SR_AD_SENSORS_DATA")
						if subRecData.AnalogSensorFieldExists1 == "1" {
							exportPacket.AnSensors = append(exportPacket.AnSensors, AnSensor{1, subRecData.AnalogSensor1})
						}

						if subRecData.AnalogSensorFieldExists2 == "1" {
							exportPacket.AnSensors = append(exportPacket.AnSensors, AnSensor{2, subRecData.AnalogSensor2})
						}

						if subRecData.AnalogSensorFieldExists3 == "1" {
							exportPacket.AnSensors = append(exportPacket.AnSensors, AnSensor{3, subRecData.AnalogSensor3})
						}
						if subRecData.AnalogSensorFieldExists4 == "1" {
							exportPacket.AnSensors = append(exportPacket.AnSensors, AnSensor{4, subRecData.AnalogSensor4})
						}
						if subRecData.AnalogSensorFieldExists5 == "1" {
							exportPacket.AnSensors = append(exportPacket.AnSensors, AnSensor{5, subRecData.AnalogSensor5})
						}
						if subRecData.AnalogSensorFieldExists6 == "1" {
							exportPacket.AnSensors = append(exportPacket.AnSensors, AnSensor{6, subRecData.AnalogSensor6})
						}
						if subRecData.AnalogSensorFieldExists7 == "1" {
							exportPacket.AnSensors = append(exportPacket.AnSensors, AnSensor{7, subRecData.AnalogSensor7})
						}
						if subRecData.AnalogSensorFieldExists8 == "1" {
							exportPacket.AnSensors = append(exportPacket.AnSensors, AnSensor{8, subRecData.AnalogSensor8})
						}
					case *egts.SrAbsCntrData:
						logger.Debugf("Разбор подзаписи EGTS_SR_ABS_CNTR_DATA")

						switch subRecData.CounterNumber {
						case 110:
							// Три младших байта номера передаваемой записи (идет вместе с каждой POS_DATA).
							binary.BigEndian.PutUint32(packetIdBytes, subRecData.CounterValue)
							exportPacket.PacketID = subRecData.CounterValue
						case 111:
							// один старший байт номера передаваемой записи (идет вместе с каждой POS_DATA).
							tmpBuf := make([]byte, 4)
							binary.BigEndian.PutUint32(tmpBuf, subRecData.CounterValue)

							if len(packetIdBytes) == 4 {
								packetIdBytes[3] = tmpBuf[3]
							} else {
								packetIdBytes = tmpBuf
							}

							exportPacket.PacketID = binary.LittleEndian.Uint32(packetIdBytes)
						}
					case *egts.SrLiquidLevelSensor:
						logger.Debugf("Разбор подзаписи EGTS_SR_LIQUID_LEVEL_SENSOR")
						sensorData := LiquidSensor{
							SensorNumber: subRecData.LiquidLevelSensorNumber,
							ErrorFlag:    subRecData.LiquidLevelSensorErrorFlag,
						}

						switch subRecData.LiquidLevelSensorValueUnit {
						case "00", "01":
							sensorData.ValueMm = subRecData.LiquidLevelSensorData
						case "10":
							sensorData.ValueL = subRecData.LiquidLevelSensorData * 10
						}

						exportPacket.LiquidSensors = append(exportPacket.LiquidSensors, sensorData)
					}
				}

				if isPkgSave {
					if err := store.Save(&exportPacket); err != nil {
						logger.Error(err)
					}
				}
			}

			resp, err := createPtResponse(&pkg, resultCode, serviceType, srResponsesRecord)
			if err != nil {
				logger.Errorf("Ошибка сборки ответа: %v", err)
				goto Received
			}
			_, _ = conn.Write(resp)

			logger.Debugf("Отправлен пакет EGTS_PT_RESPONSE: %X", resp)

			if len(srResultCodePkg) > 0 {
				_, _ = conn.Write(srResultCodePkg)
				logger.Debugf("Отправлен пакет EGTS_SR_RESULT_CODE: %X", resp)
			}
		case egts.PtResponsePacket:
			logger.Printf("Тип пакета EGTS_PT_RESPONSE")
		}

	}
}