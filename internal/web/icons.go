package web

import (
	"fmt"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The filled icons are one sprite, static/img/gi.svg: a <symbol> per icon (game-icons.net, see static/img/CREDITS.md), drawn with
// <use href> by the gi_icon template. A page carries a short reference per icon instead of its drawing, and the browser keeps the
// sprite for a year (its ?v= changes with its content). The maps below say which icon draws what; icons_test.go checks that every
// name is in the sprite and that every symbol of the sprite is used.

const iconSprite = "img/gi.svg"

// iconHref is the address of an icon of the sprite, for <use href>.
func iconHref(name string) string { return staticURL(iconSprite) + "#" + name }

// kindIcons draw the kind of a unit: a swordwoman (infantry, the default), a cavalry charge, a wolf head (beast), an orc head
// (monster) and a dragon head (monstrous cavalry).
var kindIcons = map[core.UnitKind]string{
	core.KindInfantry: "swordwoman", core.KindCavalry: "cavalry", core.KindBeast: "wolf-head", core.KindMonster: "orc-head",
	core.KindMonstrousCavalry: "dragon-head",
}

func kindIcon(k core.UnitKind) string {
	if icon, ok := kindIcons[k]; ok {
		return icon
	}
	return kindIcons[core.KindInfantry]
}

// roleLookView is the insignia of a unit's place in the company; a warrior has none (Icon empty).
type roleLookView struct{ Icon, Label string }

// roleLooks are three chevrons for the leader, two for a sergeant, one for another hero.
var roleLooks = map[core.UnitRole]roleLookView{
	core.RoleLeader: {"rank-3", "Leader"}, core.RoleSergeant: {"rank-2", "Sergeant"}, core.RoleHero: {"rank-1", "Hero"},
}

// roleLook takes a core.UnitRole, or its name as the pages hold it.
func roleLook(r any) roleLookView { return roleLooks[core.UnitRole(fmt.Sprint(r))] }

// gearLookView is how a piece of gear is drawn: its icon and its tone, the colour of the eq-tone-* class (one of the item classes'
// --eq-* colours, gold or other, the muted tone).
type gearLookView struct{ Icon, Tone string }

// gearLooks are by item class (core.EquipmentClass) and by item key (core.IconFor). An item drawn more precisely than by its class
// keeps a class's colour, so the colours still group the gear.
var gearLooks = map[string]gearLookView{
	// item classes
	"mount": {"horse-head", "mount"}, "melee": {"sword-brandish", "melee"}, "two_handed": {"battle-axe", "melee"},
	"spear": {"spears", "spear"}, "lance": {"mounted-knight", "lance"}, "shield": {"viking-shield", "shield"},
	"armour": {"kevlar", "armour"}, "bow": {"high-shot", "bow"}, "crossbow": {"crossbow", "crossbow"},
	"throwing": {"thrown-daggers", "throwing"}, "missile": {"striking-clamps", "missile"}, "banner": {"flying-flag", "banner"},
	"instrument": {"horn", "banner"}, "consumable": {"remedy", "consumable"}, "other": {"diamond", "other"},
	// items with a drawing of their own
	"horn": {"horn", "banner"}, "drum": {"drum", "banner"}, "badge": {"medal", "banner"},
	"generic-small": {"swap-bag", "other"}, "generic-large": {"cardboard-box-closed", "other"},
	"backpack": {"backpack", "mount"}, "barding": {"lorc-horse-head", "mount"}, "cloak": {"cape", "mount"}, "map": {"treasure-map", "mount"},
	"boots": {"boots", "mount"}, "rope": {"rope-coil", "mount"}, "belt": {"belt", "mount"}, "pelt": {"animal-hide", "mount"},
	"warg": {"wolf-head", "mount"}, "camel": {"camel", "mount"}, "goat": {"goat", "mount"}, "armoured-horse": {"lorc-horse-head", "mount"},
	"steed": {"horse-head", "bow"}, "pony": {"horse-head", "armour"}, // the same horse, told apart by colour
	"axe": {"magic-axe", "melee"}, "hammer": {"warhammer", "melee"}, "hand-and-a-half": {"blade-drag", "melee"},
	"twin-blades": {"crossed-swords", "melee"}, "claws": {"claw-slashes", "melee"}, "torch": {"torch", "melee"}, "vial": {"poison-bottle", "melee"},
	"glaive": {"glaive", "spear"}, "halberd": {"spears", "gold"},
	"thrown-spear": {"thrown-spear", "throwing"}, "smoke-bomb": {"unlit-bomb", "throwing"}, "chain": {"crossed-chains", "throwing"},
	"whip": {"whip", "throwing"}, "talisman": {"gem-pendant", "throwing"}, "ring": {"ring", "throwing"}, "scroll": {"tied-scroll", "throwing"},
	"light-armour": {"leather-vest", "armour"}, "heavy-armour": {"breastplate", "armour"}, "blowpipe": {"cannon-shot", "armour"}, "snare": {"wolf-trap", "armour"},
	"small-shield": {"round-shield", "shield"}, "pavise": {"dragon-shield", "shield"},
	"brew": {"beer-stein", "bow"}, "herbs": {"herbs-bundle", "bow"}, "cram": {"sandwich", "bow"},
	"orb": {"dragon-orb", "crossbow"},
}

// gearLook is the look of an item class (a core.EquipmentClass, or its name as the pages hold it) or of an item key; a key the
// app does not know (another rules file) gets a plain parcel.
func gearLook(key any) gearLookView {
	if l, ok := gearLooks[fmt.Sprint(key)]; ok {
		return l
	}
	return gearLookView{"parcel", "other"}
}
