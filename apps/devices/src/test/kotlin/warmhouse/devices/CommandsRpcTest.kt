package warmhouse.devices

import com.google.protobuf.Timestamp
import io.grpc.ManagedChannelBuilder
import io.grpc.Status
import io.grpc.StatusRuntimeException
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import warmhouse.v1.*
import java.time.Instant
import java.util.UUID
import java.util.concurrent.TimeUnit

/** Public generated RPCs against the real Compose Devices, broker and simulator. */
class CommandsRpcTest {
    @Test
    fun `manual on and new off commands retain boolean oneof and converge to completed snapshots`() {
        val channel = ManagedChannelBuilder.forTarget(System.getenv("DEVICES_GRPC_ADDR") ?: "devices:9090").usePlaintext().build()
        try {
            val stub = DevicesServiceGrpc.newBlockingStub(channel)
            var previousSeq = 0L
            var previousId = ""
            for (enabled in listOf(true, false)) {
                val id = UUID.randomUUID().toString()
                val expires = Instant.now().plusSeconds(60)
                val input = CommandInput.newBuilder().setCommandId(id).setCapabilityId("cap-heating")
                    .setExpiresAt(Timestamp.newBuilder().setSeconds(expires.epochSecond).setNanos(expires.nano))
                    .setParameters(Parameters.newBuilder().setOperation("set").putArguments("enabled", Value.newBuilder().setBooleanValue(enabled).build())).build()
                val accepted = stub.withDeadlineAfter(10, TimeUnit.SECONDS).submitManualCommand(SubmitManualCommandRequest.newBuilder().setDeviceId("dev-thermostat").setCommand(input).build())
                assertFalse(accepted.replayed)
                assertEquals("accepted", accepted.command.status)
                assertTrue(accepted.command.hasAcceptanceSeq())
                assertFalse(accepted.command.hasResultVersion())
                assertFalse(accepted.command.hasReasonCode())
                assertEquals("user-1", accepted.command.userId)
                assertEquals("manual", accepted.command.source)
                assertEquals(input.parameters, accepted.command.parameters)
                assertEquals(input.expiresAt, accepted.command.expiresAt)
                assertEquals(Value.ValueCase.BOOLEAN_VALUE, accepted.command.parameters.getArgumentsOrThrow("enabled").valueCase)
                assertEquals(enabled, accepted.command.parameters.getArgumentsOrThrow("enabled").booleanValue)
                assertTrue(accepted.command.acceptanceSeq > previousSeq)
                assertNotEquals(previousId, id)
                val request = GetCommandRequest.newBuilder().setDeviceId("dev-thermostat").setCommandId(id).build()
                val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(10)
                var current: Command
                do {
                    current = stub.withDeadlineAfter(3, TimeUnit.SECONDS).getCommand(request)
                    if (current.status == "succeeded") break
                    Thread.sleep(50)
                } while (System.nanoTime() < deadline)
                assertEquals("succeeded", current.status, "command_id=$id last=$current")
                assertEquals(id, current.commandId)
                assertEquals("dev-thermostat", current.deviceId)
                assertEquals("cap-heating", current.capabilityId)
                assertEquals(input.parameters, current.parameters)
                assertEquals(Value.ValueCase.BOOLEAN_VALUE, current.parameters.getArgumentsOrThrow("enabled").valueCase)
                assertEquals(enabled, current.parameters.getArgumentsOrThrow("enabled").booleanValue)
                assertTrue(current.hasResultVersion())
                assertEquals(1L, current.resultVersion)
                assertTrue(current.hasReasonCode())
                assertEquals("COMPLETED", current.reasonCode)
                assertEquals(accepted.command.createdAt, current.createdAt)
                assertEquals(accepted.command.acceptanceSeq, current.acceptanceSeq)
                assertFalse(current.hasRuleId()); assertFalse(current.hasRuleVersion()); assertFalse(current.hasTriggerId())
                val otherDevice = assertThrows(StatusRuntimeException::class.java) {
                    stub.withDeadlineAfter(3, TimeUnit.SECONDS).getCommand(request.toBuilder().setDeviceId("missing").build())
                }
                assertEquals(Status.Code.NOT_FOUND, otherDevice.status.code)
                println("command_id=$id boolean_oneof=$enabled acceptance_seq=${current.acceptanceSeq} result_version=${current.resultVersion} status=${current.status}")
                previousSeq = current.acceptanceSeq; previousId = id
            }
            val unsupported = assertThrows(StatusRuntimeException::class.java) {
                stub.withDeadlineAfter(3, TimeUnit.SECONDS).submitScenarioCommand(SubmitScenarioCommandRequest.getDefaultInstance())
            }
            assertEquals(Status.Code.UNIMPLEMENTED, unsupported.status.code)
        } finally { channel.shutdownNow().awaitTermination(5, TimeUnit.SECONDS) }
    }
}
