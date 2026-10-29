# Writes the fixtures of testdata/valid and the inputs of testdata/invalid (the .want files come from `go test -update`).
# Run: python3 -I internal/core/lean/testdata/gen_fixtures.py
import json,os,copy
HERE=os.path.dirname(os.path.abspath(__file__))
out=HERE+'/valid'
CAMPAIGN={
 "company":{"maxModels":15,"leaders":1,"sergeants":2,"heroStart":{"might":1,"will":1,"fate":1},"startingInfluence":0,"equipmentSlots":{"small":1,"large":1}},
 "recruitment":{"die":"d6","adjust":{"maxPerAttempt":3,"direction":"up","costPerPoint":1},"rare":{"injuredCount":True,"deadCount":False,"promotedCount":False}},
 "influence":{"gains":{"participating":2,"draw":1,"win":2}},
 "progression":{"threshold":5,
   "warrior":{"roll":"d6","table":[{"roll":"1-2","key":"noChange","name":"No Change"},{"roll":"3-5","key":"promoted","name":"Promoted"},{"roll":6,"key":"heroInTheMaking","name":"A Hero in the Making"}],"noPromotion":{"courage":1,"intelligence":1,"maximum":3}},
   "hero":{"roll":"2d6","rollsPerProgression":2,"choose":1}},
 "injuries":{"roll":"2d6",
   "warrior":[{"roll":"2-3","key":"dead","name":"Dead"},{"roll":"4-5","key":"injured","name":"Injured"},{"roll":"6-11","key":"fullRecovery","name":"Full Recovery"},{"roll":12,"key":"lessonLearned","name":"Lesson Learned","grantsExperience":{"dice":"d3"}}],
   "hero":[{"roll":2,"key":"dead","name":"Dead"},{"roll":"3-11","key":"fullRecovery","name":"Full Recovery"},{"roll":12,"key":"heroic","name":"Heroic Recovery","statChanges":[{"stat":"fate","amount":1}]}]},
 "experience":{"gains":[{"key":"participated","label":"Participating in the scenario","amount":1},{"key":"mvp","label":"MVP","amount":1,"dice":"d3"}]},
 "breakPoint":{"default":{"percent":50,"of":"startingModels"},"threshold":{"comparison":"gt"},"whenBroken":{"courageTest":{"dice":"2d6"}}},
 "rating":{"hero":{"wargear":{"threshold":{"stats":["attack","wounds"],"sumAtLeast":3}}}},
 "scenarioSelection":{"die":"d2","tables":[{"roll":[1,1],"scenarios":["Seize Ground","Chance Encounter"]},{"roll":[2,2],"scenarios":["Hold the Line","Rescue"]}]},
}
SCEN=[{"name":n,"description":"..."} for n in ["Seize Ground","Chance Encounter","Hold the Line","Rescue"]]
def doc(**kw):
    d={"schemaVersion":2,"name":"Fixture","version":"1.0","extends":"Middle-Earth Strategy Battle Game - 2024","campaign":copy.deepcopy(CAMPAIGN),"scenarios":copy.deepcopy(SCEN)}
    d.update(kw); return d
def w(name,d):
    json.dump(d,open(f'{out}/{name}.json','w'),indent=2,ensure_ascii=False); open(f'{out}/{name}.json','a').write('\n')
w('minimal',doc())
w('company',doc(rules=[{"name":"Foresight of the Seer","definition":"The hero may re-roll ...","source":"Arnor Hero Upgrade"}],
  companies=[{"name":"Arnor","alignment":"good","description":"Arnor was once a great realm ...",
   "epigraph":{"quote":"Leave all that can be spared behind. We travel light.","text":"As Arnor fell, ..."},
   "oath":{"vow":"I solemnly vow","text":"Heirs of Arnor, I take command of this company ... called {name}. ..."},
   "start":{"gold":4,"units":[{"unit":"Warrior of Arnor","count":4},{"unit":"Ranger of Arnor","count":3}]},
   "reinforcements":{"influence":3,"rows":[{"roll":1,"nothing":True,"blocks":True},{"roll":"2-3","unit":"Warrior of Arnor"},{"roll":4,"unit":"Ranger of Arnor"},{"roll":5,"unit":"Ranger of Arnor","with":["Spear"]},{"roll":6,"unit":"Knight of Arnor"}]},
   "advancements":[{"from":"Warrior of Arnor","to":["Knight of Arnor"]}],
   "heroUpgrades":[{"name":"Foresight of the Seer","flavour":"It is the hero's sworn duty ..."}],
   "promotions":[{"from":"Ranger of Arnor","on":"heroInTheMaking","becomes":"Ranger of the North","statGain":False,"pathChoice":"any","text":"Should a Ranger of Arnor Warrior achieve a Hero in the Making result ..."}]}]))
w('company_extends',doc(companies=[
  {"name":"Arnor","alignment":"good","description":"...","start":{"gold":4,"units":[{"unit":"Ranger of Arnor","count":3}]},
   "reinforcements":{"influence":3,"rows":[{"roll":"1-3","unit":"Ranger of Arnor"},{"roll":"4-6","unit":"Knight of Arnor","rare":1}]},"heroUpgrades":[]},
  {"name":"Elite Arnor","extends":"Arnor","alignment":"good","maxModels":12,"faction":"Arnor","description":"An Elite company."}]))
w('unit_forms',doc(companies=[{"name":"Osgiliath & Ithilien","alignment":"good","description":"...","faction":"Gondor",
   "start":{"gold":0,"units":[{"unit":"Ranger of Ithilien","count":1}]},
   "reinforcements":{"influence":3,"rows":[{"roll":"1-3","unit":"Ranger of Gondor"},{"roll":4,"unit":"Ranger of Ithilien"},{"roll":"5-6","unit":"Tookish Hunter"}]},
   "units":[{"name":"Ranger of Ithilien","extends":"Ranger of Gondor","addRules":["Loyal to the Captain"],"note":"A Ranger of Ithilien is a Ranger of Gondor with the Loyal to the Captain special rule at no additional cost."},
            {"name":"Tookish Hunter","extends":"Hobbit Archer","with":["Bow"]}],"ownPath":{"units":["Ranger of Gondor"]},"heroUpgrades":[]}],
  units=[{"name":"Angmar Warg Rider","tts":{"foot":"Angmar Orc Warrior","states":False}},
         {"name":"Mordor Hound","profile":{"movement":9,"fight":3,"shoot":"-","strength":3,"defence":4,"attack":1,"wounds":1,"courage":5,"intelligence":3,"race":["Orc"],"unitType":["Warrior","Infantry"]},"points":20,"aka":["Hound of Mordor"],"note":"A brand-new unit with a full definition"}]))
w('gear',doc(gear=[
  {"name":"Badge of Courage","kind":"equipment","size":"small","influence":2,"rating":5,"definition":"A model with a Badge of Courage gains the Fearless special rule.","grants":["Fearless"]},
  {"name":"Spear","kind":"wargear","type":"spear","influence":1,"rating":{"warrior":1,"hero":5}},
  {"name":"Bow","aka":["Dwarf Bow","Uruk-hai Bow","Orc Bow","Short Bow"],"kind":"wargear","type":"bow","influence":1,"rating":{"warrior":1,"hero":5}},
  {"name":"Dunharrow Spear","kind":"wargear","extends":"Spear","influence":1,"rating":{"warrior":1,"hero":5},"who":{"units":["Rider of Rohan"],"hero":False},"limit":2,"definition":"A model armed with a Dunharrow Spear ...","classes":["melee","spear"]},
  {"name":"Backpack","kind":"equipment","size":"large","influence":1,"rating":0,"grantsSlots":{"small":3}},
  {"name":"Glaive","kind":"wargear","influence":1,"rating":1},
  {"name":"Elven-made Glaive","kind":"wargear","extends":"Glaive","who":{"race":["Elf"]},"modifiers":["elven"]},
  {"name":"Stand Fast Banner","kind":"equipment","auras":[{"radius":6,"kind":"banner","affects":"friendly","when":"always","los":True,"tip":"..."}],"tts":{"name":"Banner"}}]))
w('path',doc(rules=[{"name":"Spy","tag":"Active","definition":"At the beginning of the game ..."},{"name":"Gatekeeper","tag":"Active"},{"name":"Heroic Move Mastery","definition":"A hero with this mastery ..."}],
  paths=[{"name":"Path of the Tactician","kind":"heroic","dice":"2d6","description":"...",
   "maximums":{"fight":{"maxIncrease":3},"shoot":{"best":3},"attack":{"max":2},"wounds":{"max":2}},"mastery":"Heroic Move Mastery","heroicAction":None,"signature":["will"],
   "rows":[{"roll":2,"options":[{"rule":"Spy"}]},{"roll":3,"options":[{"rule":"Gatekeeper"}]},{"roll":4,"options":[{"stat":"fight"}]},{"roll":"5-8","options":[{"stat":"attack","amount":1},{"stat":"wounds","amount":1}]},
          {"roll":"9-11","options":[{"oneOf":[{"stat":"might"},{"stat":"fate"}]}]},{"roll":12,"options":[{"rule":{"name":"Master Tactician","definition":"Inline definition hoisted ..."}}]}]}]))
w('rules',doc(rules=[
  {"name":"At 'em, Lads!","source":"Hobbit Heroes","target":{"appliesTo":"enemy-keyword","keywords":["Hobbit"],"match":"all","text":"enemy {X} models"}},
  {"name":"Hatred","overrides":True,"parameter":{"kind":"keyword","picks":"company"},"target":{"appliesTo":"enemy-keyword","text":"enemy models with the {X} keyword"},"definition":"Hatred (X) ..."},
  {"name":"Stand Fast","auras":[{"radius":6,"kind":"command","affects":"friendly","when":"always","los":False,"tip":"Friendly models within 6\" ..."}]},
  {"name":"Lead by Example","auras":[{"radius":6,"kind":"banner","affects":"friendly","when":"conditional","los":True,"keywords":["Orc","Troll"],"tip":"After slaying 3 enemy Heroes or Monsters ..."}]},
  {"name":"Quiet Step","minor":True,"definition":"A minor rule.","aka":["Silent Step"]}],
  auras={"kinds":[{"id":"banner","label":"Banner","colour":"yellow","rgb":"F0E442","default":True}],"groups":[{"id":"always","label":"Rules always in force","default":True}]},
  magic={"powers":{"Fortify Spirit":{"range":"12\"","casting":"3+"}},"castingRule":"..."}))
w('auras',doc(rules=[
  {"name":"Stand Fast","auras":[{"mergeLabels":["With Me!","Loose"],"from":"hero","radius":6,"kind":"command","affects":"friendly","when":"always","los":True,"keywords":["Orc"],"tip":"Friendly models within 6\" ..."}]},
  {"name":"Heroic Parade","definition":"The hero declares ...","auras":[{"label":"At the Double","from":"heroicAction","radius":6,"kind":"command","affects":"friendly","when":"declared","los":False}]},
  {"name":"Voice of Doom","definition":"Friendly models within 12\" ...","auras":[{"radius":12,"kind":"command","affects":"friendly","when":"always","los":True,"replaces":["Stand Fast","At the Double"]}]},
  {"name":"Lay of the Wood","definition":"An Elf may sing ...","auras":[
     {"label":"Lay of the Wood: Hymn of Elbereth","radius":12,"kind":"support","affects":"friendly","when":"target","los":False},
     {"label":"Lay of the Wood: Lay of Gondolin","radius":3,"kind":"support","affects":"friendly","when":"declared","los":False}]},
  {"name":"Keen Sight (Falcon)","label":"Keen Sight","definition":"Enemy models within 12\" of a Falcon ...","auras":[{"radius":12,"kind":"enemy","affects":"enemy","when":"always","los":False}]},
  {"name":"Fury","definition":"Fury (X) ...","parameter":{"kind":"keyword"},"auras":[{"from":"power","radius":6,"kind":"support","affects":"friendly","when":"cast","los":False,"keywords":["Orc"]}]},
  {"name":"Fright","definition":"Fright (X) ...","parameter":{"kind":"keyword"},
     "target":{"appliesTo":"enemy-keyword","text":"enemy models with the {X} keyword"},"bareTarget":{"appliesTo":"enemy-any","text":"enemy models in general"}},
  {"name":"Lordly Quality","definition":"Friendly models ...","target":{"appliesTo":"friendly-keyword","text":"friendly models that share the bearer's faction keyword"}}],
  gear=[{"name":"Banner","auras":[{"radius":3,"kind":"banner","affects":"friendly","when":"always","los":True}]},
        {"name":"Signal Horn","aka":["Company Signal"],"kind":"equipment","size":"small","influence":2,"rating":5,"definition":"A horn ...","auras":[
           {"radius":24,"kind":"command","affects":"friendly","when":"always","group":"large","los":False},
           {"label":"Signal Horn: Stand Fast +3\"","radius":9,"kind":"command","affects":"friendly","when":"declared","bearer":"Hero","los":False}]}],
  auras={"kinds":[{"id":"banner","label":"Banner","colour":"yellow","rgb":"F0E442","default":True},{"id":"command","label":"Command","colour":"red","rgb":"CC3311"}],
         "groups":[{"id":"always","label":"Rules always in force","default":True},{"id":"large","label":"Large rings"},{"id":"declared","label":"Declared"},{"id":"cast","label":"Cast"},{"id":"target","label":"Target"}]},
  magic={"powers":{"Fortify Spirit":{"range":"12\"","casting":"3+","auras":[{"radius":6,"kind":"support","affects":"friendly","when":"cast","los":False}]},"Wrath of Bruinen":{"range":"self","casting":"2+"}},
         "castingRule":"...","floor":2,"maxPerPower":2}))
w('scenario',doc(scenarios=[{"name":"Seize Ground","outline":"A short lead-in.","usesRules":["Maelstrom of Battle"],"description":"As two companies ...","armies":"All available members of each Battle Company.","layout":"...","startingPositions":"...","objectives":"The game lasts until ...","specialRules":[],"roles":[],"rewards":[],
   "scoring":[{"key":"objectiveMarkers","side":"both","kind":"eachTiers","item":"Objective Marker","count":5,"options":[
     {"text":"You score 1 Victory Point if you have more models within 3\" than your opponent.","points":1},{"text":"If you have at least twice as many models ...","points":2},{"text":"If you are the only player ...","points":3}]}]},
   {"name":"Chance Encounter","description":"...","scoring":[{"key":"ownHeroSurvives","secret":True,"nomination":{"from":"ownHeroes","allowLeader":"onlyHero"},"options":[{"text":"x","points":1}]},{"key":"terrainPiece","secret":True,"nomination":{"from":"terrain","where":"opponentHalf"},"options":[{"text":"y","points":1}]}]},{"name":"Hold the Line","description":"...","side":"defender"},{"name":"Rescue","description":"..."}]))
w('vanilla_section',doc(vanilla={"pins":[{"kind":"hero","name":"Peregrin Took","occurrence":2}],
   "units":{"Hunter Orc Warrior":{"race":["Orc"],"faction":["Gundabad"],"unitType":["Warrior","Infantry"]}},
   "mounts":{"Warg":{"race":["Warg"],"faction":["Mordor"]}},
   "options":{"notSold":["banner","war horn","war drum"],"exceptions":[{"unit":"Cave Troll","option":"Hand-and-a-half hammer","readAs":"Hand-and-a-half Weapon"},{"unit":"Cave Troll","option":"Troll Spear","ownItem":"Troll Spear"},{"unit":"Ranger of the North","option":"Armour","granted":True}]}},
   ))
w('limits_followers',doc(limits=[{"id":"bow","type":"bow","property":"all","maximumPercentage":33.33,"roundUp":True,"classes":["bow","crossbow"],"label":"bows (heroes included)"}],
   creatures={"rules":{"maxPerHero":1}},wanderers={"rules":{"maxAtOnce":1}}))
w('company_typed',doc(rules=[{"name":"Foresight of the Seer","definition":"The hero may re-roll ...","source":"Fornost Hero Upgrade"}],
  gear=[{"name":"Spear","kind":"wargear","type":"spear","influence":1,"rating":1},{"name":"Armour","kind":"wargear","type":"armour","influence":1,"rating":1},{"name":"Bow","kind":"wargear","type":"bow","influence":1,"rating":1}],
  limits=[{"id":"bow","type":"bow","property":"all","maximumPercentage":33.33,"roundUp":True,"classes":["bow","crossbow"],"label":"bows (heroes included)"}],
  companies=[{"name":"Fornost","alignment":"good","description":"...",
   "notes":[{"section":"reinforcements","text":"What the page says about the chart."},{"section":"startingCompany","text":"A note.","about":["Tookish Hunter"]}],
   "start":{"gold":2,"units":[{"unit":"Ranger of Arnor","role":"leader","with":["Spear"]},{"unit":"Tookish Hunter","count":2,"role":"heroInTheMaking"},{"unit":"Warrior of Arnor","count":3}]},
   "reinforcements":{"influence":3,"rows":[{"roll":1,"nothing":True,"blocks":True},
      {"roll":2,"options":[{"unit":"Ranger of Arnor","rare":1},{"unit":"Tookish Hunter","rare":1}],"text":"Any of the profiles of the starting company that was lost."},
      {"roll":3,"unit":"Warrior of Arnor","pick":True,"rare":2},{"roll":4,"choose":"Choice of above","scope":"above"},{"roll":5,"unit":"Ranger of Arnor","with":["Spear"]},{"roll":6,"special":True}]},
   "special":{"rows":[{"roll":"1-3","unit":"Ranger of Arnor"},{"roll":"4-6","unit":"Knight of Arnor","rare":1}],"buy":[{"unit":"Cave Troll","influence":15,"rare":1,"pick":True}]},
   "advancements":[{"from":"Ranger of Arnor","with":["Spear"],"to":["Knight of Arnor",{"unit":"Ranger of the North","with":["Armour"]}]}],
   "units":[{"name":"Tookish Hunter","extends":"Hobbit Archer","with":["Bow"],"addKeywords":["Hobbit"],"note":"A Hobbit Archer with the option selected."},
            {"name":"Cave Troll","types":{"add":["Cavalry"],"remove":["Infantry"]},"models":2,"includes":[{"name":"Warrior of Arnor","points":3}],
             "composite":{"parts":[{"role":"rider","unit":"Warrior of Arnor","count":2,"with":["Spear"]},{"role":"mount","mount":"Horse"}],"counts":{"slots":3,"maxModels":3},
                          "onePhysicalCard":True,"casualty":{"injuredWhen":"allRidersRemoved","table":"warrior","wholeModel":True},"rules":["Fearless"]}}],
   "heroUpgrades":[{"name":"Foresight of the Seer","flavour":"...","who":{"keywords":["Hobbit"],"units":["Ranger of Arnor"],"text":"Hobbits"},"note":"Why."}],
   "promotions":[{"from":"Ranger of Arnor","on":"heroInTheMaking","becomes":"Ranger of the North","keep":["Spear"],"drop":["Armour"],"forgo":True,"text":"..."}],
   "heroes":{"who":{"units":["Ranger of Arnor","Tookish Hunter"]},"noStartGain":["Ranger of the North"],"recruitAsHero":{"might":1,"will":1,"fate":1}},
   "recruit":{"fallback":[{"unit":"Cave Troll","when":"wouldExceedMaxModels","charts":["reinforcements","specialChart"]},{"unit":"Cave Troll","when":"memberMissing","choose":"replacementOfMissingMember","swapRollOn":"specialChart","anyRoll":True}],
              "replace":[{"unit":"Ranger of the North","max":1,"roles":["leader","sergeant"],"when":"slain"},{"chart":"reinforcements","roll":"2-6","alternative":"Hobbit Archer","playerChoice":True}],
              "balance":[{"keyword":"Elf","notMoreThan":"Man","onExceed":"chooseLowerResult"}]},
   "limits":{"exempt":[{"unit":"Rider of Rohan","limits":["bow"],"text":"Riders do not count."},{"item":"Spear","limits":["bow"]}],"override":{"bow":50},"company":[{"keyword":"Elf","percent":33,"text":"No more than 33%."}]},
   "ownPath":{"units":["Cave Troll"]},
   "substitutions":[{"appliesTo":"Models","target":{"role":"leader"},"treatedAs":"Faramir","rule":"Loyal to the Captain","text":"Models treat the Leader as Faramir."},{"appliesTo":"Guards","target":{"hero":True,"withRule":"Fearless"},"treatedAs":"The Master","text":"..."}],
   "exchanges":[{"remove":"Hatred (Rohan)","add":"Hatred (Hobbit)","text":"..."}],
   "granted":[{"rule":"Fearless","who":{"hero":True,"text":"All Hero models"},"cost":0,"text":"All Hero models gain Fearless."}],
   "elvenWeaponry":{"units":["Ranger of Arnor"],"hero":True},
   "specialRules":["A sentence with no typed form."]}]))
print(sorted(os.listdir(out)))

# ---- invalid catalogue (bind level) ----
inv=HERE+'/invalid'
def wi(name,d,raw=None):
    open(f'{inv}/{name}.json','w').write(raw if raw is not None else json.dumps(d,indent=2,ensure_ascii=False)+'\n')
def path_rows(over):
    rows=[{"roll":r,"options":[{"stat":"fight"}]} for r in range(2,13)]
    for k,v in over.items(): rows[k-2]=v
    return rows
def base_company():
    return {"name":"Arnor","alignment":"good","description":"...","start":{"gold":4,"units":[{"unit":"Ranger of Arnor","count":3}]},
            "reinforcements":{"influence":3,"rows":[{"roll":"1-3","unit":"Ranger of Arnor"},{"roll":"4-6","unit":"Knight of Arnor"}]},"heroUpgrades":[]}
d=doc(companies=[base_company()]); d['companies'][0]['strat']=d['companies'][0].pop('start'); wi('unknown_key',d)
d=doc(gear=[{"name":"Bow","kind":"wargaer"}]); wi('vocabulary',d)
d=doc(rules=[{"name":"Stand Fast","auras":[{"radius":6,"kind":"banners","affects":"friendly","when":"always","los":False}]}]); wi('aura_kind',d)
d=doc(); d['scenarios'][0]['scoring']=[{"key":"objectiveMarkers","options":[{"text":"x","points":"one"}]}]; wi('scenario_points',d)
d=doc(); d['campaign']['injuries']['roll']="2d6x"; wi('dice_error',d)
d=doc(); d['campaign']['recruitment']['adjust']['maxPerAttempt']=40; wi('param_range',d)
d=doc(); del d['campaign']['company']['heroStart']; wi('param_missing',d)
d=doc(); d['campaign']['progression']['hero']['choose']=0; d['campaign']['breakPoint']['threshold']['comparison']="ge"; wi('param_vocabulary',d)
d=doc(paths=[{"name":"Path of Test","rows":path_rows({2:{"roll":2,"options":[{"stat":"figth"}]}})}]); wi('path_stat',d)
raw=json.dumps(doc(companies=[base_company()]),indent=2)
wi('syntax_error',None,raw.replace('"name": "Arnor",','"name": "Arnor"',1).replace('"alignment"','"alignment" "good" ,',1)[:])
wi('duplicate_key',None,'{"schemaVersion": 2, "name": "A", "name": "B"}')

# ---- lean model gaps of the rules (auras on items and powers, labels, replaces, bare target, casting limits) ----
ring={"radius":6,"kind":"command","affects":"friendly","when":"always","los":False}
d=doc(rules=[{"name":"Stand Fast","auras":[dict(ring)]},{"name":"Voice of Doom","definition":"...","auras":[dict(ring,radius=12,replaces=["Stnad Fast","Voice of Doom","Heroic Rally"])]}]); wi('aura_replaces',d)
d=doc(rules=[{"name":"Lay of the Wood","definition":"...","auras":[dict(ring),dict(ring,radius=3,label="Lay of the Wood")]}]); wi('aura_labels',d)
d=doc(rules=[{"name":"Stand Fast","auras":[dict(ring,group="lage",mergeLabels=["With Me!",""])]}],
      gear=[{"name":"Banner","auras":[{**ring,"from":"heroicAction"}]}],
      auras={"kinds":[{"id":"command","label":"Command"}],"groups":[{"id":"always","label":"Always"},{"id":"large","label":"Large"}]}); wi('aura_group_from',d)
d=doc(rules=[{"name":"Lordly Quality","definition":"...","target":{"appliesTo":"friendly-keyword"}}]); wi('rule_target_friendly',d)
d=doc(rules=[{"name":"Fright","definition":"...","bareTarget":{"appliesTo":"enemy-keyword"}},
             {"name":"Dread","definition":"...","parameter":{"kind":"keyword"},"bareTarget":{"appliesTo":"enemy-keyword"}}]); wi('bare_target',d)
d=doc(magic={"powers":{"Fortify Spirit":{"range":"12\"","casting":"1+"}},"floor":2,"maxPerPower":0}); wi('magic_floor',d)
d=doc(magic={"powers":{"Fortify Spirit":{"range":"12\"","casting":"3+"}},"floor":7}); wi('magic_range',d)

# ---- name resolution catalogue ----
d=doc(companies=[base_company()]); d['companies'][0]['start']['units'][0]['unit']="Ranger of Arnr"; wi('unknown_unit',d)
d=doc(companies=[base_company()]); d['companies'][0]['reinforcements']['rows'][0]['with']=["Speer"]; wi('unknown_option',d)
c=base_company(); c['heroUpgrades']=[{"name":"Foresight of the Seer"}]; wi('unknown_rule',doc(companies=[c]))
wi('duplicate_name',doc(gear=[{"name":"Bow","kind":"wargear","influence":1,"rating":1},{"name":"Sling","kind":"wargear","influence":1,"rating":1},{"name":"Bow","kind":"wargear","influence":1,"rating":1}]))
wi("silent_shadowing",doc(rules=[{"name":"Fearless","definition":"My own Fearless."}]))
wi('missing_definition',doc(rules=[{"name":"Fearless","note":"annotation"},{"name":"Foo"}]))
wi('alias_shadow',doc(units=[{"name":"Mordor Hound","profile":{"movement":9,"fight":3,"shoot":"-","strength":3,"defence":4,"attack":1,"wounds":1,"courage":5,"intelligence":3,"race":["Orc"],"unitType":["Warrior","Infantry"]},"points":20},{"name":"Ranger of Mordor","extends":"Ranger of Gondor","aka":["Ranger of Gondor"]}]))
wi('extends_missing',doc(units=[{"name":"Ranger of Ithilien","extends":"Ranger of Gondr"}]))
wi('extends_loop',doc(gear=[{"name":"A","extends":"B"},{"name":"B","extends":"C"},{"name":"C","extends":"A"}]))
wi('annotation_of_nothing',doc(units=[{"name":"Nobody Special","tts":{"foot":"Ranger of Gondor"}}]))
c=base_company(); c['rules']=[{"name":"Shared Wisdom","definition":"One text."}]
wi('hoist_conflict',doc(rules=[{"name":"Shared Wisdom","definition":"Another text."}],companies=[c]))
d=doc(); d['campaign']['scenarioSelection']['tables'][0]['scenarios'][1]="Chance Encountr"; wi('scenario_table_unknown',d)
wi('unit_without_name',doc(units=[{"note":"nothing"}]))

# ---- structure catalogue ----
c=base_company(); c['reinforcements']['rows']=[{"roll":"1-2","unit":"Ranger of Arnor","nothing":True},{"roll":3},{"roll":"4-5","unit":"Ranger of Arnor","blocks":True},{"roll":6,"nothing":True,"rare":1}]
wi('chart_rows',doc(companies=[c]))
c=base_company(); c['heroes']={"who":{"except":["Ranger of Arnor"]}}; wi('officers',doc(companies=[c]))
c=base_company(); c['start']['units']=[{"unit":"Ranger of Arnor","count":16}]; wi('too_many_models',doc(companies=[c]))
d=doc(); d['scenarios'][0]['scoring']=[{"key":"objectiveMarkers","options":[{"text":"a","points":1}]},{"key":"objectiveMarkers","options":[{"text":"","points":2}]}]; wi('scenario_scoring',d)
d=doc(); d['scenarios'][0]['scoring']=[{"key":"a","nomination":{"from":"mountain"}},{"key":"b","nomination":{"from":"terrain","allowLeader":"onlyHero"}},{"key":"c","nomination":{"from":"ownHeroes","where":"opponentHalf"}},{"key":"d","nomination":{"allowLeader":"onlyHero"}}]; wi('scenario_nomination',d)
wi('path_option',doc(paths=[{"name":"Path of Test","rows":path_rows({2:{"roll":2,"options":[{"stat":"fight","power":"Fortify Spirit"}]},3:{"roll":3,"options":[{"maxTotal":2}]},4:{"roll":4,"options":[{"stat":"fight","maxTotal":1}]}})}]))
wi('path_text',doc(rules=[{"name":"Spy","tag":"Active","definition":"At the beginning of the game."}],paths=[{"name":"Path of Test","rows":path_rows({2:{"roll":2,"options":[{"rule":"Spy"}],"text":"Spy (Active) - At the beginning of the game."},3:{"roll":3,"options":[{"rule":"Spy"}],"text":"Something else."}})}]))
wi('vanilla_pins',doc(vanilla={"pins":[{"kind":"hero","name":"Peregrin Took","occurrence":2},{"kind":"hero","name":"Peregrin Took","occurrence":3},{"kind":"hero","name":"Nobody At All","occurrence":1}]}))
wi('rule_target',doc(rules=[{"name":"Fury","definition":"d","target":{"appliesTo":"enemy-keyword"}}]))
wi("gear_kind",doc(gear=[{"name":"Mystery Item","definition":"A new item without a kind.","influence":1,"rating":1}]))

# ---- chart and parameter catalogue ----
c=base_company(); c['reinforcements']['rows']=[{"roll":"1-2","unit":"Ranger of Arnor"},{"roll":"2-3","unit":"Ranger of Arnor"},{"roll":"5-5","unit":"Knight of Arnor"},{"roll":"3-2","unit":"Knight of Arnor"},{"roll":7,"unit":"Knight of Arnor"}]
wi('bad_chart',doc(companies=[c]))
c=base_company(); c['reinforcements']['rows']=[{"roll":"1-5","unit":"Ranger of Arnor"}]; c['special']={"rows":[{"roll":"2-4","unit":"Knight of Arnor"}]}
wi('chart_missing',doc(companies=[c]))
d=doc(); d['campaign']['injuries']['warrior']=d['campaign']['injuries']['warrior'][:2]; d['campaign']['injuries']['hero'][2]['roll']=11; wi('injury_table',d)
wi('path_rows',doc(paths=[{"name":"Path of Test","dice":"2d6","rows":[{"roll":2,"options":[{"stat":"fight"}]},{"roll":"3-6","options":[{"stat":"fight"}]},{"roll":"8-12","options":[{"stat":"fight"}]}]},
                           {"name":"Path of Dice","dice":"d6","rows":[{"roll":"1-3","options":[{"stat":"fight"}]}]}]))
d=doc(); d['campaign']['scenarioSelection']={"die":"d6","tables":[{"roll":[1,2],"scenarios":["Seize Ground"]},{"roll":[3,5],"scenarios":["Seize Ground","Rescue","Hold the Line","Chance Encounter","Rescue","Seize Ground"]},{"roll":[5,6],"scenarios":[]}]}; wi('scenario_tables',d)
d=doc(); d['campaign']['progression']['hero']['choose']=3; d['campaign']['progression']['hero']['rollsPerProgression']=2; d['campaign']['company']['leaders']=3; d['campaign']['company']['sergeants']=6; d['campaign']['company']['maxModels']=5; wi('param_cross',d)
d=doc(); d['campaign']['recruitment']['die']="2d6"; d['campaign']['aftermathEffects']=[{"key":"rendezvous"},{"key":"vantagePoint","range":[1,-1]}]; d['campaign']['progression']['warrior']['table'][0]['key']="noChang"; wi('param_dice',d)

# ---- header and version ----
d=doc(); d['schemaVersion']=1; wi('version',d)
d=doc(); del d['schemaVersion']; wi('version_missing',d)
wi('old_format',None,json.dumps({"name":"Battle Companies","version":"1.1","extends":"MESBG","campaign":{},"data":{"companies":[]}},indent=2)+"\n")

# ---- a file with twelve independent mistakes, one issue each ----
c=base_company()
c['colour']="red"                                              # 1 unknown field
c['alignment']="nuetral"                                       # 2 vocabulary
c['start']['units'][0]['unit']="Ranger of Arnr"                # 3 unknown unit
c['reinforcements']['rows']=[{"roll":"1-3","unit":"Ranger of Arnor","with":["Speer"]},{"roll":"4-5","unit":"Knight of Arnor"}]   # 4 unknown option, 5 chart missing 6
c['heroUpgrades']=[{"name":"Foresight of the Seer"}]           # 6 unknown rule
d=doc(companies=[c],
  gear=[{"name":"Bow","kind":"wargear","influence":1,"rating":1},{"name":"Bow","kind":"wargear","influence":1,"rating":1}],   # 7 duplicate name
  rules=[{"name":"Fearless","definition":"My own."},{"name":"Foo"},                                                           # 8 shadowing, 9 missing definition
         {"name":"Stand Fast","auras":[{"radius":6,"kind":"banners","affects":"friendly","when":"always"}]}])                    # 10 aura kind
d['campaign']['recruitment']['adjust']['maxPerAttempt']=40     # 11 range
d['campaign']['injuries']['roll']="2d6x"                       # 12 dice
wi('many_mistakes',d)

# ---- company catalogue ----
c=base_company(); c['reinforcements']['rows']=[{"roll":1,"special":True,"choose":"Choice","scope":"any"},{"roll":"2-3","unit":"Ranger of Arnor","pick":False,"text":"words"},{"roll":4,"nothing":True,"pick":True},{"roll":5,"unit":"Ranger of Arnor","options":[{"unit":"Knight of Arnor"}]},{"roll":6,"nothing":True,"with":["Spear"]}]
c['special']={"rows":[{"roll":"1-5","special":True},{"roll":6,"unit":"Knight of Arnor"}]}
wi('chart_row_kinds',doc(companies=[c]))
c=base_company()
c['advancements']=[{"from":"Ranger of Arnr","to":["Knight of Arnor",{"unit":"Ranger of Gondr","with":["Speer"]}]}]
c['heroUpgrades']=[{"name":"Foresight","who":{"units":["Ranger of Arnr"]}}]
c['units']=[{"name":"Mounted Ranger","extends":"Ranger of Arnor","composite":{"parts":[{"role":"rider","unit":"Ranger of Arnr","with":["Speer"]}],"rules":["Fearles"]},"includes":[{"name":"Knight of Arnr","points":3}]}]
c['recruit']={"fallback":[{"unit":"Ranger of Arnr","when":"wouldExceedMaxModels"}],"replace":[{"unit":"Ranger of Arnor"},{"chart":"reinforcements","alternative":"Hobbit Archr","roll":"2-6"},{"unit":"Knight of Arnor","roles":["leader"],"alternative":"Hobbit Archer"}],
             "balance":[{"keyword":"Elv","notMoreThan":"Hobit"}]}
c['limits']={"exempt":[{"limits":["bow"]},{"unit":"Rider of Rohan","limits":["bows"]}],"override":{"cavalry":50},"company":[{"keyword":"Elv","percent":33}]}
c['substitutions']=[{"appliesTo":"Models","target":{"role":"leader"},"treatedAs":"Faramir","rule":"Loyal to the Captan","text":"x"}]
c['granted']=[{"rule":"Fearles","who":{"text":"All"},"text":"x"}]
wi('company_typed',doc(companies=[c],limits=[{"id":"bow","type":"bow","property":"all","maximumPercentage":33.33,"roundUp":True,"classes":["bow","crossbow"],"label":"bows"}]))

c=base_company(); c['elvenWeaponry']={"units":["Ranger of Arnr"],"race":["Elfe"],"except":["Knight of Arnr"]}
wi('elven_weaponry',doc(companies=[c]))
