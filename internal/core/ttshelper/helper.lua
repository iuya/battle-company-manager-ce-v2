-- BC names helper for Tabletop Simulator (Battle Company Manager).
-- Reads the Notebook tab "ArmyNames" (JSON v1 or v2 pasted from the app) and gives the figures of an
-- army spawned by the MESBG FTC mod the name and description of their card; a v2 payload also
-- carries aura rings, which the helper draws as vector lines on the figures.
-- Own code, common subset of Lua 5.1 and 5.2 (TTS runs MoonSharp): no goto, no integer
-- division, no bit operators. It only reads objects and sets name, description, GM notes, a
-- tag and vector lines; it never moves, clones or destroys anything.

local TAB_NAME = "ArmyNames"
local TAG = "BCNamed"
local NOTE_PREFIX = "bc-unit:"
local MAX_BODY = 262144
local ROW = 0.55 -- the spawner puts rows 1.1 apart; z is rounded to half a row

local ORANGE = {1, 0.5, 0}
local GREEN = {0.2, 1, 0.2}
local RED = {1, 0, 0}

local unpack_ = table.unpack or unpack

-- Rings: constants and state --------------------------------------------

local KINDS = {"banner", "command", "support", "enemy", "utility"}
local KIND_INDEX = {banner = 1, command = 2, support = 3, enemy = 4, utility = 5}
local KIND_LETTER = {banner = "B", command = "C", support = "S", enemy = "E", utility = "U"}
-- Okabe-Ito, colour-blind friendly; the app may send other colours in the payload
local PALETTE = {banner = "F0E442", command = "E69F00", support = "56B4E9", enemy = "D55E00", utility = "CC79A7"}
local THICKNESS = {banner = 0.045, command = 0.045, support = 0.045, enemy = 0.06, utility = 0.045}
local STEPS = 48 -- segments of a ring (49 points, closed)
local EDGE = 0.0225 -- half a line: the outer edge of the line lands on the range
local HEIGHT = 0.1 -- local height of a ring, plus a step per kind
local HEIGHT_STEP = 0.015
local SAME_RADIUS_GAP = 0.03
local MAX_PER_FIGURE = 6
local MAX_RINGS = 200
local MAX_RADIUS = 48
local CHUNK = 10 -- figures drawn per frame
local REDRAW_DELAY = 3 -- the figures' own scripts clear their lines after 2 seconds
local DEFAULT_BASE = 0.49

-- state.figs: guid -> {id, b, rings = {{r, k, w, n}}, drawn}; saved with the game
local state = {on = true, hidden = {}, pal = {}, figs = {}}
local recent = {} -- guid -> record of a figure destroyed a moment ago (a state change follows)
local warnedOverwrite = false

local BUTTON_RINGS = 2
local BUTTON_FIRST_KIND = 4 -- 0 Apply, 1 Re-apply, 2 Rings, 3 Redraw, then one per kind

local function paletteColor(kind)
    return state.pal[kind] or PALETTE[kind]
end

local function hexRGB(hex)
    if type(hex) ~= "string" or not hex:match("^%x%x%x%x%x%x$") then
        return nil
    end
    return {tonumber(hex:sub(1, 2), 16) / 255, tonumber(hex:sub(3, 4), 16) / 255, tonumber(hex:sub(5, 6), 16) / 255}
end

function onSave()
    local figs = {}
    for guid, rec in pairs(state.figs) do
        figs[guid] = {id = rec.id, rings = rec.rings}
    end
    return JSON.encode({v = 1, on = state.on, hidden = state.hidden, pal = state.pal, figs = figs})
end

local function loadState(saved)
    if type(saved) ~= "string" or saved == "" then
        return
    end
    local ok, data = pcall(JSON.decode, saved)
    if not ok or type(data) ~= "table" then
        return
    end
    if data.on == false then
        state.on = false
    end
    if type(data.hidden) == "table" then
        for k, v in pairs(data.hidden) do
            if KIND_INDEX[k] and v == true then
                state.hidden[k] = true
            end
        end
    end
    if type(data.pal) == "table" then
        for k, v in pairs(data.pal) do
            if KIND_INDEX[k] and hexRGB(v) then
                state.pal[k] = v
            end
        end
    end
    if type(data.figs) == "table" then
        for guid, rec in pairs(data.figs) do
            if type(guid) == "string" and type(rec) == "table" and type(rec.rings) == "table" then
                state.figs[guid] = {id = rec.id, rings = rec.rings, drawn = false}
            end
        end
    end
end

local function later(fn, seconds)
    if Wait ~= nil and Wait.time ~= nil then
        Wait.time(fn, seconds)
    else
        fn()
    end
end

local function nextFrame(fn)
    if Wait ~= nil and Wait.frames ~= nil then
        Wait.frames(fn, 1)
    else
        fn()
    end
end

local function ringsLabel()
    return state.on and "Rings: on" or "Rings: off"
end

local function kindLabel(kind)
    if state.hidden[kind] then
        return "(" .. KIND_LETTER[kind] .. ")"
    end
    return KIND_LETTER[kind]
end

local function editLabel(index, label)
    if self.editButton ~= nil then
        pcall(function()
            self.editButton({index = index, label = label})
        end)
    end
end

function onLoad(saved)
    loadState(saved)
    self.createButton({
        label = "Apply names",
        click_function = "applyNames",
        function_owner = self,
        tooltip = "Name and describe the selected figures (all figures when none is selected) from the ArmyNames notebook tab; draws their aura rings",
        position = {0, 0.6, -0.6},
        rotation = {0, 180, 0},
        width = 1500,
        height = 300,
        font_size = 200
    })
    self.createButton({
        label = "Re-apply",
        click_function = "reapplyNames",
        function_owner = self,
        tooltip = "Send the names and rings again, also to figures that are already named (changed data); first removes the rings drawn before",
        position = {0, 0.6, -0.2},
        rotation = {0, 180, 0},
        width = 1500,
        height = 240,
        font_size = 160
    })
    self.createButton({
        label = ringsLabel(),
        click_function = "toggleRings",
        function_owner = self,
        tooltip = "Show or hide all aura rings",
        position = {0.4, 0.6, 0.25},
        rotation = {0, 180, 0},
        width = 700,
        height = 220,
        font_size = 130
    })
    self.createButton({
        label = "Redraw",
        click_function = "redrawRings",
        function_owner = self,
        tooltip = "Draw the aura rings again (the mod's own circle keys remove them from a figure)",
        position = {-0.4, 0.6, 0.25},
        rotation = {0, 180, 0},
        width = 700,
        height = 220,
        font_size = 130
    })
    for i, kind in ipairs(KINDS) do
        self.createButton({
            label = kindLabel(kind),
            click_function = "toggleKind" .. KIND_LETTER[kind],
            function_owner = self,
            tooltip = "Show or hide the " .. kind .. " rings",
            position = {0.8 - (i - 1) * 0.4, 0.6, 0.65},
            rotation = {0, 180, 0},
            color = hexRGB(paletteColor(kind)),
            font_color = {0, 0, 0},
            width = 320,
            height = 260,
            font_size = 170
        })
    end
    if next(state.figs) ~= nil then
        later(function()
            runRedraw(false)
        end, REDRAW_DELAY)
    end
end

function applyNames(obj, color)
    runHelper(color, false)
end

function reapplyNames(obj, color)
    runHelper(color, true)
end

-- Messages ---------------------------------------------------------------

local function tell(color, text, rgb)
    if color and color ~= "" and type(broadcastToColor) == "function" then
        broadcastToColor(text, color, rgb)
    else
        broadcastToAll(text, rgb)
    end
end

local function tellAll(text, rgb)
    broadcastToAll(text, rgb)
end

-- Text helpers -----------------------------------------------------------

-- Lower case, single spaces, no line breaks; also the accented capitals of Latin-1 (the app
-- writes keys with a Unicode-aware lower case, string.lower only knows ASCII).
local function fold(s)
    if type(s) ~= "string" then
        return ""
    end
    s = s:gsub("\195([\128-\158])", function(c)
        local b = c:byte()
        if b == 151 then
            return "\195" .. c
        end
        return "\195" .. string.char(b + 32)
    end)
    s = s:lower()
    s = s:gsub("[%c]+", " ")
    s = s:gsub("%s+", " ")
    s = s:gsub("^ ", "")
    s = s:gsub(" $", "")
    return s
end

local function trim(s)
    return (s:gsub("^%s+", ""):gsub("%s+$", ""))
end

local function isTagged(obj)
    return obj.hasTag ~= nil and obj.hasTag(TAG) == true
end

-- The GM note names the company unit: "bc-unit:<u>", and "bc-unit:<u>:foot" or
-- "bc-unit:<u>:mount" for the extra figures that share the rider's id. When several figures
-- of the payload still share that id (the parts of a composite unit), "/<n>" is added, n being
-- their position among them.
local function noteId(u, x)
    local id = tostring(u)
    if type(x) == "string" and x ~= "" then
        id = id .. ":" .. x
    end
    return id
end

local function unitOf(obj)
    local notes = obj.getGMNotes and obj.getGMNotes() or ""
    if type(notes) ~= "string" then
        return nil
    end
    return notes:match("^bc%-unit:(%d+:?%a*/?%d*)$")
end

local function coord(p, name, index)
    if p == nil then
        return 0
    end
    return p[name] or p[index] or 0
end

-- Notebook ---------------------------------------------------------------

local function findTab(color)
    local tabs = getNotebookTabs() or {}
    local first = nil
    for _, tab in ipairs(tabs) do
        if tab.title == TAB_NAME then
            if tab.color ~= nil and color ~= nil and tab.color == color then
                return tab
            end
            if first == nil then
                first = tab
            end
        end
    end
    return first
end

local function readPayload(color)
    local tab = findTab(color)
    if tab == nil then
        tell(color, "Notebook tab '" .. TAB_NAME .. "' not found. Create it and paste the names from the app.", RED)
        return nil
    end
    local body = trim(tab.body or "")
    if body == "" then
        tell(color, "The notebook tab '" .. TAB_NAME .. "' is empty. Paste the names from the app.", RED)
        return nil
    end
    if #body > MAX_BODY then
        tell(color, "The notebook tab '" .. TAB_NAME .. "' is too large (" .. #body .. " characters). Paste only the names of one company.", RED)
        return nil
    end
    local ok, data = pcall(JSON.decode, body)
    if not ok or type(data) ~= "table" then
        tell(color, "The text in '" .. TAB_NAME .. "' is not valid JSON (" .. #body .. " characters; the tab may have been cut). Copy the names from the app again.", RED)
        return nil
    end
    if data.v ~= 1 and data.v ~= 2 then
        tell(color, "This helper is older than the app (names format " .. tostring(data.v) .. "). Download the helper again from the app.", RED)
        return nil
    end
    if type(data.figures) ~= "table" then
        tell(color, "The names in '" .. TAB_NAME .. "' have no figures. Copy the names from the app again.", RED)
        return nil
    end
    return data
end

-- Candidates -------------------------------------------------------------

local function selectedObjects(color)
    if color == nil or Player == nil or Player[color] == nil then
        return {}
    end
    local ok, list = pcall(function()
        return Player[color].getSelectedObjects()
    end)
    if ok and type(list) == "table" then
        return list
    end
    return {}
end

local function isSelf(obj)
    return obj == self or (obj.getGUID ~= nil and self.getGUID ~= nil and obj.getGUID() == self.getGUID())
end

-- Spawn-grid order: rows by ascending z (rounded), then ascending x, then the old order.
local function sortGrid(objs)
    local rows = {}
    for i, obj in ipairs(objs) do
        local p = obj.getPosition()
        rows[obj] = {math.floor(coord(p, "z", 3) / ROW + 0.5), coord(p, "x", 1), i}
    end
    table.sort(objs, function(a, b)
        local ka, kb = rows[a], rows[b]
        if ka[1] ~= kb[1] then
            return ka[1] < kb[1]
        end
        if ka[2] ~= kb[2] then
            return ka[2] < kb[2]
        end
        return ka[3] < kb[3]
    end)
end

local function apply(obj, entry)
    obj.setName(entry.n or "")
    obj.setDescription(entry.d or "")
    obj.setGMNotes(NOTE_PREFIX .. entry.id)
    if not isTagged(obj) then
        obj.addTag(TAG)
    end
end

local function hasStates(obj)
    if obj.getStates == nil then
        return false
    end
    local ok, states = pcall(function()
        return obj.getStates()
    end)
    return ok and type(states) == "table" and #states > 0
end

-- Rings ------------------------------------------------------------------

local function colorOf(c)
    return {coord(c, "r", 1), coord(c, "g", 2), coord(c, "b", 3)}
end

local function sameColor(a, b)
    return math.abs(a[1] - b[1]) < 0.01 and math.abs(a[2] - b[2]) < 0.01 and math.abs(a[3] - b[3]) < 0.01
end

local function scaleOf(obj)
    local ok, sc = pcall(function()
        return obj.getScale()
    end)
    local x = ok and sc ~= nil and coord(sc, "x", 1) or 1
    if type(x) ~= "number" or x <= 0 then
        return 1
    end
    return x
end

-- Half of the larger side of the figure's bounds, as the mod's own circle measures: the base
-- plus any overhang. A "b" field in an old payload or saved state is ignored.
local function boundsRadius(obj)
    local ok, b = pcall(function()
        return obj.getBoundsNormalized()
    end)
    if ok and type(b) == "table" and type(b.size) == "table" then
        local r = math.max(coord(b.size, "x", 1), coord(b.size, "z", 3)) / 2
        if r > 0 then
            return r
        end
    end
    return DEFAULT_BASE
end

-- A line is ours when it has the colour of a kind and that kind's thickness (divided by the
-- scale of the figure, as we draw it). The mod's circle is green and 0.05 thick.
local function isOurs(line, scale)
    if type(line) ~= "table" or type(line.thickness) ~= "number" then
        return false
    end
    local color = colorOf(line.color)
    local thick = line.thickness * scale
    for _, kind in ipairs(KINDS) do
        if math.abs(thick - THICKNESS[kind]) < 0.002 then
            if sameColor(color, hexRGB(PALETTE[kind])) then
                return true
            end
            local own = state.pal[kind] and hexRGB(state.pal[kind])
            if own ~= nil and sameColor(color, own) then
                return true
            end
        end
    end
    return false
end

local function readLines(obj)
    if obj.getVectorLines == nil then
        return nil
    end
    local ok, lines = pcall(function()
        return obj.getVectorLines()
    end)
    if not ok then
        return nil
    end
    if type(lines) ~= "table" then
        return {}
    end
    return lines
end

-- setVectorLines replaces every line of the object: keep the lines that are not ours.
local function putLines(obj, rec, scale, ours)
    local existing = readLines(obj)
    local keep = {}
    local removed = 0
    if existing ~= nil then
        for _, line in ipairs(existing) do
            if isOurs(line, scale) then
                removed = removed + 1
            else
                keep[#keep + 1] = line
            end
        end
        if #ours == 0 and removed == 0 then
            rec.drawn = false
            return
        end
    else
        if #ours == 0 and not rec.drawn then
            return
        end
        if not warnedOverwrite then
            warnedOverwrite = true
            tellAll("The lines of a figure cannot be read here: drawing rings may remove the mod's own circle (press its key again).", ORANGE)
        end
    end
    for _, line in ipairs(ours) do
        keep[#keep + 1] = line
    end
    pcall(function()
        obj.setVectorLines(keep)
    end)
    rec.drawn = #ours > 0
end

local function ringLine(ring, radius, scale, height)
    local points = {}
    local r = radius / scale
    for i = 0, STEPS do
        local a = (i % STEPS) * 2 * math.pi / STEPS
        points[#points + 1] = {r * math.cos(a), height / scale, r * math.sin(a)}
    end
    return {points = points, color = hexRGB(paletteColor(ring.k)), thickness = THICKNESS[ring.k] / scale}
end

local function drawFigure(guid)
    local rec = state.figs[guid]
    local obj = getObjectFromGUID(guid)
    if rec == nil or obj == nil then
        return 0
    end
    local scale = scaleOf(obj)
    local ours = {}
    if state.on then
        local base = boundsRadius(obj)
        local used = {}
        for _, ring in ipairs(rec.rings) do
            if not state.hidden[ring.k] then
                local key = tostring(ring.r)
                local gap = (used[key] or 0) * SAME_RADIUS_GAP
                used[key] = (used[key] or 0) + 1
                local height = HEIGHT + HEIGHT_STEP * KIND_INDEX[ring.k]
                ours[#ours + 1] = ringLine(ring, ring.r + base - EDGE + gap, scale, height)
            end
        end
    end
    putLines(obj, rec, scale, ours)
    return #ours
end

-- Draw the figures in chunks, one chunk per frame; done(figures, rings) at the end.
local function drawMany(guids, done)
    local i, figures, rings = 1, 0, 0
    local function step()
        local last = math.min(i + CHUNK - 1, #guids)
        for j = i, last do
            local n = drawFigure(guids[j])
            if n > 0 then
                figures = figures + 1
                rings = rings + n
            end
        end
        i = last + 1
        if i <= #guids then
            nextFrame(step)
        elseif done ~= nil then
            done(figures, rings)
        end
    end
    step()
end

local function sortedGuids()
    local list = {}
    for guid in pairs(state.figs) do
        list[#list + 1] = guid
    end
    table.sort(list)
    return list
end

local function summary(figures, rings)
    if figures > 0 then
        tellAll("Rings drawn for " .. figures .. " figures (" .. rings .. " rings).", GREEN)
    end
end

function runRedraw(say)
    drawMany(sortedGuids(), say and summary or nil)
end

-- Re-apply: take the rings of the figures we know off first.
local function removeAllRings()
    for guid, rec in pairs(state.figs) do
        local obj = getObjectFromGUID(guid)
        if obj ~= nil then
            putLines(obj, rec, scaleOf(obj), {})
        end
    end
    state.figs = {}
end

local function totalRings(except)
    local n = 0
    for guid, rec in pairs(state.figs) do
        if guid ~= except then
            n = n + #rec.rings
        end
    end
    return n
end

-- The rings of a payload figure: valid ones only.
local function parseRings(f)
    local list = {}
    if type(f) == "table" and type(f.a) == "table" then
        for _, a in ipairs(f.a) do
            if type(a) == "table" and type(a.r) == "number" and a.r > 0 and a.r <= MAX_RADIUS and KIND_INDEX[a.k] ~= nil then
                list[#list + 1] = {r = a.r, k = a.k, w = type(a.w) == "string" and a.w or "a", n = type(a.n) == "string" and a.n or ""}
            end
        end
    end
    return list
end

-- Remember the rings of a figure that was matched to an entry; returns the rings left out.
local function remember(obj, entry, cap, tally)
    if #entry.rings == 0 then
        return 0
    end
    local guid = obj.getGUID()
    local keep = {}
    local others = totalRings(guid)
    for _, ring in ipairs(entry.rings) do
        if #keep < cap and others + #keep < MAX_RINGS then
            keep[#keep + 1] = ring
        end
    end
    state.figs[guid] = {id = entry.id, b = entry.b, rings = keep, drawn = state.figs[guid] ~= nil and state.figs[guid].drawn or false}
    tally[#tally + 1] = guid
    return #entry.rings - #keep
end

function toggleRings(obj, color)
    state.on = not state.on
    editLabel(BUTTON_RINGS, ringsLabel())
    runRedraw(false)
end

function redrawRings(obj, color)
    runRedraw(true)
end

local function toggleKind(kind)
    state.hidden[kind] = not state.hidden[kind] or nil
    editLabel(BUTTON_FIRST_KIND + KIND_INDEX[kind] - 1, kindLabel(kind))
    runRedraw(false)
end

function toggleKindB() toggleKind("banner") end
function toggleKindC() toggleKind("command") end
function toggleKindS() toggleKind("support") end
function toggleKindE() toggleKind("enemy") end
function toggleKindU() toggleKind("utility") end

-- A figure replaced by another state: the entry follows, the new state's own script clears its
-- lines after two seconds, so draw after three. The names are not carried over by the mod.
function onObjectStateChange(newObj, oldGuid)
    if newObj == nil or newObj.getGUID == nil then
        return
    end
    local rec = state.figs[oldGuid] or recent[oldGuid]
    if rec == nil then
        return
    end
    local guid = newObj.getGUID()
    state.figs[oldGuid] = nil
    recent[oldGuid] = nil
    rec.drawn = false
    state.figs[guid] = rec
    later(function()
        drawFigure(guid)
    end, REDRAW_DELAY)
end

-- A destroyed figure leaves the table; it is kept a few seconds in case it is the old state of
-- a state change whose event comes after.
function onObjectDestroy(obj)
    if obj == nil or obj.getGUID == nil then
        return
    end
    local guid = obj.getGUID()
    local rec = state.figs[guid]
    if rec ~= nil then
        state.figs[guid] = nil
        recent[guid] = rec
        later(function()
            recent[guid] = nil
        end, 5)
    end
end

-- Main -------------------------------------------------------------------

function runHelper(color, reapply)
    local data = readPayload(color)
    if data == nil then
        return
    end

    if reapply then
        removeAllRings()
    end
    local withRings = data.v == 2
    local cap = MAX_PER_FIGURE
    if withRings and type(data.auras) == "table" then
        if type(data.auras.cap) == "number" and data.auras.cap >= 1 and data.auras.cap < cap then
            cap = math.floor(data.auras.cap)
        end
        if type(data.auras.kinds) == "table" then
            for _, kd in ipairs(data.auras.kinds) do
                if type(kd) == "table" and KIND_INDEX[kd.id] ~= nil and hexRGB(kd.rgb) ~= nil then
                    state.pal[kd.id] = kd.rgb
                end
            end
        end
    end
    local drawList, leftOut = {}, 0

    local entries = {}
    local byKey = {}
    local byUnit = {}
    for i, f in ipairs(data.figures) do
        -- "l" is the nickname the spawner gives the figure, "k" the same lower-cased by the app
        local match = type(f) == "table" and (type(f.l) == "string" and f.l or f.k) or nil
        if type(match) == "string" then
            local e = {k = fold(match), u = f.u, id = noteId(f.u, f.x), n = f.n, d = f.d, done = false,
                rings = withRings and parseRings(f) or {}, b = (type(f.b) == "number" and f.b > 0) and f.b or nil}
            entries[#entries + 1] = e
            byKey[e.k] = byKey[e.k] or {}
            table.insert(byKey[e.k], e)
        end
    end

    local sharing = {}
    for _, e in ipairs(entries) do
        sharing[e.id] = (sharing[e.id] or 0) + 1
    end
    local seen = {}
    byUnit = {}
    for _, e in ipairs(entries) do
        if sharing[e.id] > 1 then
            seen[e.id] = (seen[e.id] or 0) + 1
            e.id = e.id .. "/" .. seen[e.id]
        end
        if byUnit[e.id] == nil then
            byUnit[e.id] = e
        end
    end

    local selection = selectedObjects(color)
    local useSelection = #selection > 0
    local pool = useSelection and selection or getObjects()

    local named, already, extra, states = 0, 0, 0, 0
    local groups = {}
    local groupOrder = {}
    for _, obj in ipairs(pool) do
        if not isSelf(obj) then
            local u = unitOf(obj)
            local entry = u ~= nil and byUnit[u] or nil
            if isTagged(obj) then
                if entry ~= nil and not entry.done then
                    entry.done = true
                    leftOut = leftOut + remember(obj, entry, cap, drawList)
                    if reapply then
                        apply(obj, entry)
                        named = named + 1
                        if hasStates(obj) then
                            states = states + 1
                        end
                    else
                        already = already + 1
                    end
                end
            else
                local key = fold(obj.getName())
                if byKey[key] ~= nil then
                    if groups[key] == nil then
                        groups[key] = {}
                        groupOrder[#groupOrder + 1] = key
                    end
                    table.insert(groups[key], obj)
                end
            end
        end
    end

    for _, key in ipairs(groupOrder) do
        local objs = groups[key]
        sortGrid(objs)
        local free = {}
        for _, e in ipairs(byKey[key]) do
            if not e.done then
                free[#free + 1] = e
            end
        end
        for i, obj in ipairs(objs) do
            local entry = free[i]
            if entry == nil then
                extra = extra + 1
            else
                entry.done = true
                leftOut = leftOut + remember(obj, entry, cap, drawList)
                apply(obj, entry)
                named = named + 1
                if hasStates(obj) then
                    states = states + 1
                end
            end
        end
    end

    local missing = {}
    local missingOrder = {}
    local missingTotal = 0
    for _, e in ipairs(entries) do
        if not e.done then
            if missing[e.k] == nil then
                missing[e.k] = 0
                missingOrder[#missingOrder + 1] = e.k
            end
            missing[e.k] = missing[e.k] + 1
            missingTotal = missingTotal + 1
        end
    end

    -- figures named like the list that the selection left out
    local outside = 0
    if useSelection then
        local chosen = {}
        for _, obj in ipairs(selection) do
            chosen[obj] = true
        end
        for _, obj in ipairs(getObjects()) do
            if not chosen[obj] and not isSelf(obj) and not isTagged(obj) and byKey[fold(obj.getName())] ~= nil then
                outside = outside + 1
            end
        end
    end

    local total = #entries
    if named == 0 and already == 0 then
        local names = {}
        for _, obj in ipairs(pool) do
            if #names < 3 and not isSelf(obj) then
                local n = obj.getName()
                if type(n) == "string" and n ~= "" then
                    names[#names + 1] = n
                end
            end
        end
        local text = "No figure matches the names. Spawn the army first (Spawn Army) and keep the figures' names."
        if #names > 0 then
            text = text .. " First names on the table: " .. table.concat(names, " | ") .. ". The mod may have changed; tell the app's owner."
        end
        tellAll(text, ORANGE)
        return
    end

    local parts
    if named == 0 and missingTotal == 0 then
        parts = tostring(named) .. " new, " .. tostring(already) .. " already named."
    else
        parts = "Named " .. tostring(named) .. " of " .. tostring(total) .. " figures."
        if already > 0 then
            parts = parts .. " " .. tostring(already) .. " were already named."
        end
    end
    if missingTotal > 0 then
        local list = {}
        for _, k in ipairs(missingOrder) do
            list[#list + 1] = k .. " (" .. missing[k] .. ")"
        end
        parts = parts .. " Not found: " .. table.concat(list, "; ")
    end
    local color_ = (missingTotal > 0) and ORANGE or GREEN
    tellAll(parts, color_)
    if extra > 0 then
        tellAll(tostring(extra) .. " more figures than names for the same key were left alone.", ORANGE)
    end
    if outside > 0 then
        tellAll(tostring(outside) .. " figures named like the list were left alone (not selected).", ORANGE)
    end
    if states > 0 then
        tellAll(tostring(states) .. " figures have states; changing the state shows the mod's name.", ORANGE)
    end
    if #drawList > 0 then
        drawMany(drawList, summary)
    end
    if leftOut > 0 then
        tellAll(tostring(leftOut) .. " rings left out (limit " .. cap .. " per figure, " .. MAX_RINGS .. " in all).", ORANGE)
    end
end
