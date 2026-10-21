package warmhouse.devices

import com.google.gson.Gson
import com.google.gson.JsonParser
import com.google.protobuf.Timestamp
import com.google.protobuf.util.JsonFormat
import com.rabbitmq.client.*
import io.grpc.Status
import io.grpc.StatusRuntimeException
import io.grpc.stub.StreamObserver
import org.slf4j.LoggerFactory
import org.springframework.jdbc.core.JdbcTemplate
import org.springframework.stereotype.Component
import warmhouse.v1.*
import warmhouse.v1.Command
import java.time.Instant
import java.util.UUID
import java.util.concurrent.atomic.AtomicBoolean

/** Happy-path demo: database commit and broker publication are deliberately not atomic. */
@Component
class CommandsRpc(private val jdbc: JdbcTemplate) : DevicesServiceGrpc.DevicesServiceImplBase() {
    private val log = LoggerFactory.getLogger(javaClass)
    private val gson = Gson()
    private val user = System.getenv("DEMO_USER_ID") ?: "user-1"
    private lateinit var connection: Connection
    private lateinit var consumer: Channel
    private fun ts(t: Instant) = Timestamp.newBuilder().setSeconds(t.epochSecond).setNanos(t.nano).build()
    private fun instant(t: Timestamp) = Instant.ofEpochSecond(t.seconds, t.nanos.toLong())
    private fun parseCommand(json: String) = Command.newBuilder().also { JsonFormat.parser().merge(json, it) }.build()
    private fun id(value: String) = value.matches(Regex("[A-Za-z0-9_-]{1,128}"))

    fun start() {
        connection = ConnectionFactory().also {
            it.setUri(System.getenv("AMQP_URL") ?: "amqp://mvp:mvp-local@rabbitmq:5672/internal")
            it.isAutomaticRecoveryEnabled = false
        }.newConnection("devices-commands")
        consumer = connection.createChannel()
        consumer.exchangeDeclare("wh.jobs.v1", "topic", true)
        consumer.exchangeDeclare("wh.events.v1", "topic", true)
        for ((queue, key, exchange) in listOf(Triple("wh.command.requests", "command.requested", "wh.jobs.v1"), Triple("wh.devices.command-results", "command.state_changed", "wh.events.v1"))) {
            consumer.queueDeclare(queue, true, false, false, null)
            consumer.queueBind(queue, exchange, key)
        }
        consumer.basicQos(1)
        consumer.basicConsume("wh.devices.command-results", false, DeliverCallback { tag, delivery ->
            try {
                applyResult(String(delivery.body, Charsets.UTF_8))
                consumer.basicAck(delivery.envelope.deliveryTag, false)
            } catch (e: Exception) {
                // No recovery/quarantine in this MVP: keep the delivery unacknowledged and stop consumption.
                log.error("command result processing failed", e)
                consumer.basicCancel(tag)
            }
        }, CancelCallback { })
    }
    fun stop() { if (::connection.isInitialized) connection.close() }

    @Synchronized
    private fun applyResult(raw: String) {
        val r = JsonParser.parseString(raw).asJsonObject
        require(r["type"].asString == "command.state_changed" && r["schema_version"].asInt == 1)
        require(id(r["event_id"].asString))
        val occurred = Instant.parse(r["occurred_at"].asString)
        val changed = Instant.parse(r["state_changed_at"].asString)
        require(!changed.isAfter(occurred))
        require(r["status"].asString == "succeeded" && r["evidence"].asString == "operation_completed" && r["reason_code"].asString == "COMPLETED")
        require(r["result_version"].isJsonPrimitive && r["result_version"].asJsonPrimitive.isString && r["result_version"].asString == "1")
        val rows = jdbc.queryForList("SELECT snapshot FROM commands WHERE command_id=? AND device_id=?", r["command_id"].asString, r["device_id"].asString)
        require(rows.size == 1)
        val old = parseCommand(rows.single()["snapshot"].toString())
        require(old.status == "accepted" && old.capabilityId == r["capability_id"].asString && !changed.isBefore(instant(old.createdAt)))
        val updated = old.toBuilder().setStatus("succeeded").setResultVersion(1).setStateChangedAt(ts(changed)).setReasonCode("COMPLETED").build()
        jdbc.update("UPDATE commands SET snapshot=? WHERE command_id=?", JsonFormat.printer().print(updated), old.commandId)
        log.info("command result committed command_id={} event_id={}", old.commandId, r["event_id"].asString)
    }

    override fun getCommand(request: GetCommandRequest, response: StreamObserver<Command>) {
        val rows = jdbc.queryForList("SELECT snapshot FROM commands WHERE command_id=? AND device_id=?", request.commandId, request.deviceId)
        if (rows.isEmpty()) { response.onError(Status.NOT_FOUND.asRuntimeException()); return }
        response.onNext(parseCommand(rows.single()["snapshot"].toString())); response.onCompleted()
    }

    @Synchronized
    override fun submitManualCommand(request: SubmitManualCommandRequest, response: StreamObserver<SubmitCommandResponse>) {
        try {
            val input = request.command
            require(id(input.commandId) && id(request.deviceId) && input.capabilityId == "cap-heating")
            require(input.hasParameters() && input.parameters.operation == "set" && input.parameters.argumentsMap.keys == setOf("enabled"))
            require(input.parameters.argumentsMap.getValue("enabled").valueCase == Value.ValueCase.BOOLEAN_VALUE)
            require(input.hasExpiresAt() && input.expiresAt.nanos in 0..999999999)
            val expires = instant(input.expiresAt)
            val existing = jdbc.queryForList("SELECT input, snapshot FROM commands WHERE command_id=?", input.commandId)
            if (existing.isNotEmpty()) {
                val saved = SubmitManualCommandRequest.newBuilder().also { JsonFormat.parser().merge(existing.single()["input"].toString(), it) }.build()
                if (saved != request) throw Status.ALREADY_EXISTS.withDescription("command_id conflict").asRuntimeException()
                response.onNext(SubmitCommandResponse.newBuilder().setCommand(parseCommand(existing.single()["snapshot"].toString())).setReplayed(true).build()); response.onCompleted(); return
            }
            if (jdbc.queryForList("SELECT id FROM registry WHERE id=? AND owner_id=?", request.deviceId, user).isEmpty()) throw Status.NOT_FOUND.asRuntimeException()
            val now = Instant.now()
            require(expires.isAfter(now))
            val validity = (System.getenv("CONFIG_VALIDITY_MS") ?: "300000").toLong()
            require(!expires.isAfter(now.plusMillis(validity)))
            val seq = jdbc.queryForObject("SELECT nextval('command_acceptance_seq')", Long::class.java)!!
            val accepted = Command.newBuilder().setCommandId(input.commandId).setDeviceId(request.deviceId).setCapabilityId(input.capabilityId)
                .setUserId(user).setSource("manual").setParameters(input.parameters).setExpiresAt(input.expiresAt)
                .setCreatedAt(ts(now)).setStateChangedAt(ts(now)).setStatus("accepted").setAcceptanceSeq(seq).build()
            val config = mapOf("device_id" to request.deviceId, "capability_id" to input.capabilityId, "config_version" to 1,
                "adapter_key" to "partner-thermostat", "endpoint_key" to "registered-endpoint-1", "channel" to "heating",
                "issued_at" to now.toString(), "valid_until" to now.plusMillis(validity).toString())
            val job = mapOf("event_id" to UUID.randomUUID().toString(), "type" to "command.requested", "schema_version" to 1,
                "occurred_at" to now.toString(), "command_id" to input.commandId, "device_id" to request.deviceId, "capability_id" to input.capabilityId,
                "user_id" to user, "source" to "manual", "parameters" to mapOf("operation" to "set", "arguments" to mapOf("enabled" to input.parameters.argumentsMap.getValue("enabled").booleanValue)),
                "acceptance_seq" to seq.toString(), "manual_cancel_before_seq" to seq.toString(), "expires_at" to expires.toString(), "config" to config)
            jdbc.update("INSERT INTO commands(command_id,device_id,input,snapshot,acceptance_seq,manual_cancel_before_seq,expires_at,config) VALUES (?,?,?,?,?,?,?,?)",
                input.commandId, request.deviceId, JsonFormat.printer().print(request), JsonFormat.printer().print(accepted), seq, seq, expires.toString(), gson.toJson(config))
            connection.createChannel().use { publisher ->
                publisher.confirmSelect()
                val returned = AtomicBoolean(false)
                publisher.addReturnListener(ReturnCallback { returned.set(true) })
                publisher.basicPublish("wh.jobs.v1", "command.requested", true, AMQP.BasicProperties.Builder().contentType("application/json").deliveryMode(2).build(), gson.toJson(job).toByteArray(Charsets.UTF_8))
                publisher.waitForConfirmsOrDie(3000)
                check(!returned.get()) { "unroutable command" }
            }
            log.info("command accepted and published command_id={} event_id={}", input.commandId, job["event_id"])
            // Return the immutable initial snapshot even if the result has already arrived.
            response.onNext(SubmitCommandResponse.newBuilder().setCommand(accepted).build()); response.onCompleted()
        } catch (e: StatusRuntimeException) { response.onError(e)
        } catch (e: IllegalArgumentException) { response.onError(Status.INVALID_ARGUMENT.withDescription("invalid command").asRuntimeException())
        } catch (e: Exception) {
            log.error("command submission failed command_id={}", request.command.commandId, e)
            response.onError(Status.UNAVAILABLE.withDescription("command submission unavailable").asRuntimeException())
        }
    }
}
