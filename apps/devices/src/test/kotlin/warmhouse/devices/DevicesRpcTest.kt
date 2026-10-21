package warmhouse.devices

import io.grpc.ManagedChannelBuilder
import io.grpc.Status
import io.grpc.StatusRuntimeException
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import warmhouse.v1.*
import java.time.Instant
import java.util.concurrent.TimeUnit

class DevicesRpcTest {
    @Test
    fun `seed and current poll snapshot are exposed by public RPCs`() {
        val channel = ManagedChannelBuilder.forTarget(System.getenv("DEVICES_GRPC_ADDR") ?: "devices:9090").usePlaintext().build()
        try {
            val stub = DevicesServiceGrpc.newBlockingStub(channel).withDeadlineAfter(10, TimeUnit.SECONDS)
            val device = stub.getDevice(GetDeviceRequest.newBuilder().setDeviceId("dev-thermostat").build())
            assertEquals("house-home", device.houseId)
            assertEquals("type-thermostat", device.typeId)
            assertEquals("Термостат гостиной", device.name)
            assertEquals("bound", device.registrationStatus)
            val temperature = device.capabilitiesList.single { it.id == "cap-temperature" }
            assertEquals("measurement", temperature.kind)
            assertTrue(temperature.hasValueType())
            assertEquals("number", temperature.valueType)
            assertEquals("Cel", temperature.unit)
            val heating = device.capabilitiesList.single { it.id == "cap-heating" }
            assertTrue(heating.hasParametersSchema())
            assertEquals("control", heating.kind)
            val before = Instant.now()
            val page = stub.listPollConfigurations(ListPollConfigurationsRequest.getDefaultInstance())
            assertEquals(1, page.itemsCount)
            assertEquals("", page.nextCursor)
            val poll = page.getItems(0)
            val config = poll.config
            assertEquals("dev-thermostat", config.deviceId)
            assertEquals("cap-temperature", config.capabilityId)
            assertEquals(1, config.configVersion)
            assertEquals("partner-thermostat", config.adapterKey)
            assertEquals("registered-endpoint-1", config.endpointKey)
            assertEquals("temperature", config.channel)
            assertEquals("number", config.valueType)
            assertEquals("Cel", config.unit)
            val issued = Instant.ofEpochSecond(config.issuedAt.seconds, config.issuedAt.nanos.toLong())
            val until = Instant.ofEpochSecond(config.validUntil.seconds, config.validUntil.nanos.toLong())
            assertFalse(issued.isBefore(before))
            assertFalse(issued.isAfter(Instant.now()))
            assertTrue(until.isAfter(issued.plusMillis(poll.pollTtlMs.toLong())))
            assertEquals(2000, poll.pollIntervalMs)
            assertEquals(1000, poll.pollTtlMs)
            val missing = assertThrows(StatusRuntimeException::class.java) {
                stub.getDevice(GetDeviceRequest.newBuilder().setDeviceId("missing").build())
            }
            assertEquals(Status.Code.NOT_FOUND, missing.status.code)
            val missingCommand = assertThrows(StatusRuntimeException::class.java) {
                stub.getCommand(GetCommandRequest.getDefaultInstance())
            }
            assertEquals(Status.Code.NOT_FOUND, missingCommand.status.code)
        } finally { channel.shutdownNow().awaitTermination(5, TimeUnit.SECONDS) }
    }
}
