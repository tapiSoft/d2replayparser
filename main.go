package main

import (
	"github.com/urfave/cli"
	"log"
	"os"

	"encoding/json"
	"fmt"
	"github.com/dotabuff/manta"
)

// called when --dump flag is present, for debugging & learning about the replay format
func dumpContents(pe *manta.PacketEntity, pet manta.EntityEventType) error { // TODO : add some filtering options, just piping to grep/ag is pretty slow
	for k, v := range pe.Properties.KV {
		fmt.Printf("ClassName: %s\tK: %s\tV: %s\n", pe.ClassName, k, v)
	}
	return nil
}

func runParser(c *cli.Context) error {
	dump := c.Bool("dump")
	timeSeriesInterval := c.Uint("interval")

	f := os.Stdin
	fname := c.Args().First()
	if fname != "" {
		file, err := os.Open(fname)
		if err != nil {
			log.Fatalf("unable to open file: %s", err)
		}
		f = file
		defer f.Close()
	}

	p, err := manta.NewStreamParser(f)
	if err != nil {
		log.Fatalf("unable to create parser: %s", err)
	}

	if dump {
		p.OnPacketEntity(dumpContents)
		p.Start()
		return nil
	}

	netWorthData := [10][]int32{} // TODO : pair with player/hero id or something

	var gameWinner int32
	gameStarted := false
	gameEnded := false
	gameStartTime := float32(0)
	gameEndTime := float32(0)
	gameTime := int32(0)
	timeSeriesTick := int32(-1)

	p.OnPacketEntity(func(pe *manta.PacketEntity, pet manta.EntityEventType) error {
		if gameEnded {
			return nil
		}

		switch pe.ClassName {
		case "CDOTAGamerulesProxy":
			if !gameStarted {
				time, b := pe.Properties.FetchFloat32("CDOTAGamerules.m_flGameStartTime")
				if b {
					gameStarted = true
					gameStartTime = time
				} else {
					return nil
				}
			}

			winner, b := pe.Properties.FetchInt32("CDOTAGamerules.m_nGameWinner")
			if b {
				time, b := pe.Properties.FetchFloat32("CDOTAGamerules.m_flGameEndTime")
				if !b {
					panic("No end time!")
				}
				fmt.Printf("GameEndTime: %f", time)
				gameEndTime = time
				gameEnded = true
				gameWinner = winner
			} else {
				time, b := pe.Properties.FetchFloat32("CDOTAGamerules.m_fGameTime")
				if !b {
					panic(fmt.Sprintf("Time error!"))
				}
				gameTime = int32(time - gameStartTime)
			}
		case "CDOTA_DataSpectator":
			if gameStarted && gameTime >= timeSeriesTick {
				timeSeriesTick = gameTime + int32(timeSeriesInterval)
				for i := 0; i < 10; i++ {
					nw, b := pe.FetchInt32(fmt.Sprintf("m_iNetWorth.000%d", i))
					if !b {
						panic("Failed to fetch networth!")
					}
					netWorthData[i] = append(netWorthData[i], nw)
				}
			}
		}
		return nil
	})
	p.Start()

	log.Printf("Winner: %s", gameWinner)
	log.Printf("Parse Complete!\n")

	enc := json.NewEncoder(os.Stdout)

	// TODO : might want to support database connections, json can get pretty big with fine granularity time series
	jsonmap := make(map[string]interface{})
	timeSeriesData := make(map[string]interface{})
	timeSeriesData["Net Worth"] = netWorthData
	jsonmap["TimeSeries"] = timeSeriesData

	err = enc.Encode(&jsonmap)
	if err != nil {
		log.Println(err)
	}

	log.Printf("Final gametime: %f", gameTime)
	log.Printf("StartTime: %f, EndTime: %f", gameStartTime, gameEndTime)
	log.Printf("Game duration: %f", gameEndTime-gameStartTime)

	return nil
}

func main() {
	app := cli.NewApp()
	app.Name = "d2replayparse"
	app.Usage = "a Dota 2 replay parser"
	app.Version = "0.0.1"
	app.Action = runParser

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "dump",
			Usage: "Dump contents without parsing",
		},
		cli.UintFlag{
			Name:  "interval",
			Value: 60,
			Usage: "Time series recording interval in seconds",
		},
	}

	app.Run(os.Args)
}
