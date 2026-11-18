-- hyprmoncfg gives each display workspaces 1-10 of its own, numbered in blocks
-- of ten: the first display 1-10, the next 11-20.
if type(o) == "table" and type(o.workspaces) == "table" and type(o.workspaces.reconcile) == "function" then
  -- Omarchy already does this. It numbers each monitor's block, binds
  -- Super + number, brings workspaces home on hotplug, and its bar shows each
  -- monitor's own. It stays in charge, and this file adds no rules.
else
  -- Each display's block, by the selector its monitor rule uses.
  local blocks = {} -- hyprmoncfg:blocks

  local function slot_workspace(slot)
    local monitor = hl.get_active_monitor()
    local block = monitor and (blocks["desc:" .. (monitor.description or "")] or blocks[monitor.name])
    if not block then
      return tostring(slot)
    end
    return tostring(block * 10 + slot)
  end

  -- Super + number goes to that workspace on the focused display, and
  -- Super + Shift + number moves the focused window there. Both the key
  -- symbol and the key code are unbound, so an existing binding of either
  -- does not also fire.
  for slot = 1, 10 do
    local symbol = tostring(slot % 10)
    local code = "code:" .. tostring(slot + 9)
    for _, mods in ipairs({ "SUPER", "SUPER + SHIFT" }) do
      hl.unbind(mods .. " + " .. symbol)
      hl.unbind(mods .. " + " .. code)
    end
    hl.bind("SUPER + " .. code, function()
      hl.dispatch(hl.dsp.focus({ workspace = slot_workspace(slot) }))
    end, { description = "Switch to workspace " .. slot .. " on this display" })
    hl.bind("SUPER + SHIFT + " .. code, function()
      hl.dispatch(hl.dsp.window.move({ workspace = slot_workspace(slot), follow = true }))
    end, { description = "Move window to workspace " .. slot .. " on this display" })
  end

  -- hyprmoncfg:rules
end
