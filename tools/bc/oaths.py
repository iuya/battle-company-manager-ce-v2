"""Oaths sworn by the player when taking command of a Battle Company (original text). {name} is the company name the player types."""

OATHS = {
    "Minas Tirith": ("By the white walls of the Tower of Guard I take this company in my charge, to hold the line where the fires of the Enemy burn. Let us be known as {name}, and let Gondor never fall.", "For the White City"),
    "Osgiliath & Ithilien": ("Where the old bridges fall into the river, I take my place among the Rangers of the shadowed hills. We who strike from the leaves shall be called {name}, and the Enemy shall learn to fear the quiet.", "By the leaves, I vow"),
    "Fiefdoms": ("From the sea-cliffs to the vales, the lords of Gondor's fiefs send their sons, and I lead them now. Let our banners gather under the name {name}, and let no fief answer the call alone.", "For every fief"),
    "Fornost": ("The northern kings are dust, but their watch has not ended. I take up their duty on these cold hills, and our company shall be called {name}, a name the ruin will remember.", "The watch endures"),
    "Arnor": ("Heirs of Arnor, I take command of this company to keep the watch our kings began, and to carry the fury of the North against the servants of the Witch-king. From this day let us be called {name}. May that name bring dread to our foes and hope to our people.", "I solemnly vow"),
    "The Last Alliance": ("Elves and Men stand shoulder to shoulder as in the days of Gil-galad and Elendil, and I lead them. Let our oath of fellowship be sealed in the name of {name}, and let no sword be sheathed while the Dark Lord lives.", "Fellowship unbroken"),
    "Rohan": ("Riders of the Mark, I ride at your head from this day, with the wind in our banners and the grass beneath our hooves. We shall be called {name}, and the Golden Hall shall hear of our deeds.", "Ride for Rohan"),
    "Helm's Deep": ("The Hornburg stands against the dark and so shall we. Behind these walls, with the memory of Helm in our hearts, I name this company {name}, and we shall hold until the morning comes.", "We hold the Deep"),
    "Paths of the Druadan": ("Wild men and horse-lords have forgotten old quarrels, and I lead those who walk where no road runs. Let the forest remember us as {name}, and let every hidden path be ours.", "By root and hoof"),
    "Erebor & Dale": ("Under the Lonely Mountain, dwarf and man have kept the old friendship, and I am bound to keep it too. Forged in gold and in debt, we are {name}, and no dragon's cunning shall break us.", "By beard and bow"),
    "Defenders of the North": ("The North has long been forgotten by the great kingdoms, so we shall stand for ourselves. Dwarves of the hills and men of Dale, we go forth as {name}, and the road to our homes remains closed to the Shadow.", "Stand together"),
    "Lothlorien": ("Under the golden leaves, by the light of the Lady, I take up the quiet defence of this realm. We are the Galadhrim, now called {name}, and no shadow shall pass beyond our borders.", "Light guide us"),
    "Rivendell": ("In the Last Homely House, memory runs deeper than any war, and I carry its song into battle. Let this company be named {name}, for the ancient peace we guard and the long grief we endure.", "For Imladris"),
    "Halls of Thranduil": ("The King beneath the boughs shows no mercy to spiders, and I carry his will. I take his Wood Elves into the dark places and name them {name}, so that Mirkwood may breathe again.", "For the Elvenking"),
    "Durin's Folk": ("Khazad-dum was our mother and our sorrow, and I would see her halls rise again. With axe and iron I bind this clan under the name {name}, and Durin's line shall not be forgotten.", "Khazad ai-menu"),
    "Iron Hills": ("The hills ring with hammers, and the Iron Hills have never yielded a foot of stone. I take this warband of stubborn hearts and call it {name}; let any foe who tests us learn the weight of the hill-forge.", "By hammer and anvil"),
    "Lake-town": ("The lake has fed us and drowned us, and the dragon's ashes still blow in the wind. I lead the watchmen of Esgaroth as {name}, to keep our boats afloat, our coin safe, and our children warm.", "For Esgaroth"),
    "Wanderers in the Wild": ("No banner called us, no king commanded; the road itself brought us together. Friends from many peoples, we choose to be called {name}, and wherever we walk, no one in need shall face the dark alone.", "Our road, our oath"),
    "The Shire": ("I never wanted to lead anyone beyond the garden gate, yet here we stand with our pipes and our pitchforks. Let it be said in the Shire that we are {name}, and that no one threatened our green hills in vain.", "For the Shire"),
    "Mordor": ("The Great Eye commands, and I obey, for the strong rule and the weak are food. These blades shall be called {name}, and every land that remembers peace shall know the weight of our boots.", "The Eye commands"),
    "Cirith Ungol": ("Up in the dark stair, where Orc and Uruk snarl across the same fire, I have the whip and the watch. This rabble will be called {name}, and we will guard the pass or die cursing.", "None shall pass"),
    "Minas Morgul": ("The Nine rule the Tower of Sorcery, and I serve in the cold green light. Let our name be spoken in whispers, {name}, and let the living tremble as our host marches out of the vale.", "Dread is our banner"),
    "Angmar & Carn Dum": ("The Witch-king has raised the old realm of ice and ruin, and I carry his will into the snow. We who serve the black throne are {name}, and the last kingdoms of the North shall break.", "Let winter come"),
    "Burning of the Westfold": ("Fire and axe go before us, and the farms of the Westfold will feed our torches. I drive these raiders onward under the name {name}, and every hall in Rohan shall smell our smoke.", "Burn it all"),
    "Muster of Isengard": ("The White Hand marshals its Uruk-hai in Orthanc's shadow, and I am given a share of its war. We are {name}, bred for battle in the pits, and no wall of men will stop our marching.", "Saruman's will"),
    "Wolves of Isengard": ("The wargs hunger and the riders howl; the Mark has pasture and we have teeth. I lead this pack across the Fords as {name}, and the herds of Rohan shall learn who owns the night.", "Hunt together"),
    "Hill Tribes of Dunland": ("The Rohirrim took our valleys and stained our fathers' graves, and now a white wizard pays us for our vengeance. Let our hill-fires be named {name}, and let every stolen field be won back in blood.", "Remember the hills"),
    "Sharkey's Rogues": ("Old Sharkey grows fat on a ruined Shire, and a few scoundrels have been left to hold the doors. We call ourselves {name} and take what we find, like all who serve a boss who forgot honesty long ago.", "Coin and cruelty"),
    "Moria": ("Down in the Black Pit, where the dwarves once dug their shiny metal, we goblins now scurry in the dark. I gather the biggest clubs and the sharpest knives under the name {name}, and anyone who wanders our halls will not wander out.", "Down in the deep"),
    "Mirkwood": ("The forest is ours, thick with webs and watching eyes. Wargs, spiders and goblins serve me now, and I name our hunting pack {name}, that every Elf and traveller may hear the branches creak.", "The wood hungers"),
    "Harad": ("From the burning south, past the Poros, our spears march behind painted banners. I lead the sons of Harad into the northern lands as {name}, to take what the Dark Lord promised and what our swords can hold.", "By the southern sun"),
    "City of Umbar": ("The corsairs of Umbar have fought Gondor for a thousand years and I mean to settle the account. Our black ships carry a crew proud to be called {name}, and the coast shall be the first to taste our wrath.", "By sail and sword"),
    "Easterlings": ("Beyond the Sea of Rhun, our people have long served greater masters, and I now bind my chosen warriors in the Dark Lord's service. Under the banners of Rhun we are {name}, and the West will bow to the east wind.", "Rhun rises"),
    "Grand Army of the South": ("Haradrim, Khandish riders and Mahud beast-masters gather beneath one banner, and I carry their strength north. Let us be known as {name}, so that every land trembles at the thunder of our drums.", "Hear our drums"),
    "Dol Guldur": ("Within the cursed hill the Necromancer's will makes even the trees afraid. I take the hunters of his fortress and call them {name}, a name that shall fall like a shadow upon the borders of the Golden Wood.", "Into the shadow"),
    "Goblin-Town": ("Under the Misty Mountains, the Great Goblin rules with the biggest belly and the loudest voice, and I have his permission to loot. We are {name}, and we'll take anything not nailed down, plus the nails.", "Goblins forever"),
    "Gundabad": ("The old fortress of the North stirs again, and the signal towers call the Orcs to war. I forge this horde and name it {name}, for we shall make the mountains echo with ruin, as Bolg commands.", "Gundabad calls"),
}

FALLBACK = {
    "good": ("Today I take command of this Battle Company to stand against the Shadow. Henceforth let it be called {name}, and may our deeds bring hope to everyone who looks to us.", "I solemnly vow"),
    "evil": ("Today I take command of this warband in the name of the Dark Lord. Henceforth let it be called {name}, and let our enemies learn to dread it.", "So it shall be"),
}


def oath(entry):
    return {"text": entry[0], "vow": entry[1]}
