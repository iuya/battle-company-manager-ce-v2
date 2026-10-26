-- BC names helper for Tabletop Simulator (Battle Company Manager).
-- Reads the Notebook tab "ArmyNames" (JSON v1 pasted from the app) and gives the figures of an
-- army spawned by the MESBG FTC mod the name and description of their card.
-- Own code, common subset of Lua 5.1 and 5.2 (TTS runs MoonSharp): no goto, no integer
-- division, no bit operators. It only reads objects and sets name, description, GM notes and a
-- tag; it never moves, clones or destroys anything.

local TAB_NAME = "ArmyNames"
local TAG = "BCNamed"
local NOTE_PREFIX = "bc-unit:"
local MAX_BODY = 262144
local ROW = 0.55 -- the spawner puts rows 1.1 apart; z is rounded to half a row

local ORANGE = {1, 0.5, 0}
local GREEN = {0.2, 1, 0.2}
local RED = {1, 0, 0}

local unpack_ = table.unpack or unpack

function onLoad()
    self.createButton({
        label = "Apply names",
        click_function = "applyNames",
        function_owner = self,
        tooltip = "Name and describe the selected figures (all figures when none is selected) from the ArmyNames notebook tab",
        position = {0, 0.6, -0.25},
        rotation = {0, 180, 0},
        width = 1500,
        height = 340,
        font_size = 200
    })
    self.createButton({
        label = "Re-apply",
        click_function = "reapplyNames",
        function_owner = self,
        tooltip = "Send the names again, also to figures that are already named (changed data)",
        position = {0, 0.6, 0.25},
        rotation = {0, 180, 0},
        width = 1500,
        height = 260,
        font_size = 160
    })
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
    if data.v ~= 1 then
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

-- Main -------------------------------------------------------------------

function runHelper(color, reapply)
    local data = readPayload(color)
    if data == nil then
        return
    end

    local entries = {}
    local byKey = {}
    local byUnit = {}
    for i, f in ipairs(data.figures) do
        -- "l" is the nickname the spawner gives the figure, "k" the same lower-cased by the app
        local match = type(f) == "table" and (type(f.l) == "string" and f.l or f.k) or nil
        if type(match) == "string" then
            local e = {k = fold(match), u = f.u, id = noteId(f.u, f.x), n = f.n, d = f.d, done = false}
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
end
