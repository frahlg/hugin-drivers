-- Hugin workbench draft — fill this in with AI's help.
DRIVER = {
  id = "unknown_model",
  name = "Unknown Model",
  manufacturer = "Unknown",
  version = "0.1.0",
  protocols = { "modbus" },
  capabilities = { "meter" },
  tested_models = {},
  verification_status = "experimental",
  verification_notes = "Drafted via Hugin workbench; not yet verified on live hardware.",
  description = "TODO: one-sentence purpose.",
  connection_defaults = { port = 502, slave_id = 1 },
}

PROTOCOL = "modbus"

function driver_init(config)
  host.set_make("Unknown")
  host.set_sn(config.serial or "unknown")
end

function driver_poll()
  -- TODO: read registers, host.emit("meter", {...})
  return 5000
end

function driver_command(action, power_w, cmd) end
function driver_default_mode() end
function driver_cleanup() end
